package transport

import (
	"net"
	"strings"
	"sync"

	transportpb "helios/generated/transport"
	tree "helios/internal/component_tree"
)

type CommClient struct{}

const subscriptionWildcard = "*"

type Sender interface {
	ClientAddress() string
	SendTransportMessage(message *transportpb.TransportMessage)
	SendError(address string, eventName string, code transportpb.EventError_ErrorCode, message string, requestID *string)
}

type TransportService struct {
	mu            sync.RWMutex
	tree          *tree.ComponentTree
	subscriptions map[string]subscription
}

type eventKey struct {
	address   string
	eventName string
}

type subscription struct {
	id     string
	key    eventKey
	owner  string
	sender Sender
}

func NewTransportService(tree *tree.ComponentTree) *TransportService {
	return &TransportService{
		tree:          tree,
		subscriptions: make(map[string]subscription),
	}
}

func (s *TransportService) AttachTransportConnection(address string, conn net.Conn, mustBeRegistered bool) error {
	return s.tree.AttachConnection(address, conn, mustBeRegistered)
}

func (s *TransportService) DetachTransportConnection(address string) {
	s.tree.DetachConnection(address)
	s.removeSubscriptionsForClient(address)
}

func (s *TransportService) HandleTransportMessage(sender Sender, message *transportpb.TransportMessage) error {
	if message == nil {
		return nil
	}

	switch typed := message.GetMessage().(type) {
	case *transportpb.TransportMessage_EventPublish:
		s.handleEventPublish(sender, typed.EventPublish)
	case *transportpb.TransportMessage_EventRequest:
		s.handleEventRequest(sender, typed.EventRequest)
	case *transportpb.TransportMessage_EventSubscribe:
		s.handleEventSubscribe(sender, typed.EventSubscribe)
	case *transportpb.TransportMessage_EventUnsubscribe:
		s.handleEventUnsubscribe(sender, typed.EventUnsubscribe)
	default:
	}
	return nil
}

func (s *TransportService) handleEventPublish(sender Sender, message *transportpb.EventPublish) {
	if message == nil {
		return
	}
	if err := s.tree.SetEvent(message.GetAddress(), message.GetEventName(), message.GetEvent()); err != nil {
		sender.SendError(
			message.GetAddress(),
			message.GetEventName(),
			transportpb.EventError_INVALID_REQUEST,
			err.Error(),
			message.RequestId,
		)
		return
	}
	s.publishToSubscribers(message)
}

func (s *TransportService) handleEventRequest(sender Sender, message *transportpb.EventRequest) {
	if message == nil {
		return
	}

	event, ok, err := s.tree.GetEvent(message.GetAddress(), message.GetEventName())
	if err != nil {
		sender.SendError(
			message.GetAddress(),
			message.GetEventName(),
			transportpb.EventError_INVALID_REQUEST,
			err.Error(),
			message.RequestId,
		)
		return
	}
	if !ok {
		sender.SendError(
			message.GetAddress(),
			message.GetEventName(),
			transportpb.EventError_NOT_FOUND,
			"event not found",
			message.RequestId,
		)
		return
	}

	sender.SendTransportMessage(&transportpb.TransportMessage{
		Message: &transportpb.TransportMessage_EventPublish{
			EventPublish: &transportpb.EventPublish{
				Address:   message.GetAddress(),
				EventName: message.GetEventName(),
				Event:     event,
				RequestId: message.RequestId,
			},
		},
	})
}

func (s *TransportService) handleEventSubscribe(sender Sender, message *transportpb.EventSubscribe) {
	if message == nil {
		return
	}

	subscriptionID := message.GetSubscriptionId()
	if subscriptionID == "" {
		sender.SendError(message.GetAddress(), message.GetEventName(), transportpb.EventError_INVALID_REQUEST, "subscription id is empty", nil)
		return
	}
	// A wildcard address cannot be looked up in the component tree. Concrete
	// addresses still go through the usual validation; GetEvent also validates
	// that the event name is non-empty.
	if message.GetAddress() == subscriptionWildcard {
		if strings.TrimSpace(message.GetEventName()) == "" {
			sender.SendError(message.GetAddress(), message.GetEventName(), transportpb.EventError_INVALID_REQUEST, "event name is empty", &subscriptionID)
			return
		}
	} else if _, _, err := s.tree.GetEvent(message.GetAddress(), message.GetEventName()); err != nil {
		sender.SendError(message.GetAddress(), message.GetEventName(), transportpb.EventError_INVALID_REQUEST, err.Error(), &subscriptionID)
		return
	}

	s.addSubscription(subscription{
		id: subscriptionID,
		key: eventKey{
			address:   message.GetAddress(),
			eventName: message.GetEventName(),
		},
		owner:  sender.ClientAddress(),
		sender: sender,
	})
}

func (s *TransportService) handleEventUnsubscribe(sender Sender, message *transportpb.EventUnsubscribe) {
	if message == nil {
		return
	}

	subscriptionID := message.GetSubscriptionId()
	if subscriptionID == "" {
		sender.SendError("", "", transportpb.EventError_INVALID_REQUEST, "subscription id is empty", nil)
		return
	}
	s.removeSubscription(subscriptionID)
}

func (s *TransportService) addSubscription(sub subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions[sub.id] = sub
}

func (s *TransportService) removeSubscription(subscriptionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscriptions, subscriptionID)
}

func (s *TransportService) removeSubscriptionsForClient(address string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for subscriptionID, sub := range s.subscriptions {
		if sub.owner != address {
			continue
		}
		delete(s.subscriptions, subscriptionID)
	}
}

func (s *TransportService) publishToSubscribers(message *transportpb.EventPublish) {
	subscribers := s.subscribersFor(eventKey{
		address:   message.GetAddress(),
		eventName: message.GetEventName(),
	})

	for _, sub := range subscribers {
		sub.sender.SendTransportMessage(&transportpb.TransportMessage{
			Message: &transportpb.TransportMessage_EventPublish{
				EventPublish: &transportpb.EventPublish{
					Address:   message.GetAddress(),
					EventName: message.GetEventName(),
					Event:     message.GetEvent(),
					RequestId: &sub.id,
				},
			},
		})
	}
}

func (s *TransportService) subscribersFor(key eventKey) []subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subscribers := make([]subscription, 0)
	for _, sub := range s.subscriptions {
		addressMatches := sub.key.address == key.address || sub.key.address == subscriptionWildcard
		eventMatches := sub.key.eventName == key.eventName || sub.key.eventName == subscriptionWildcard
		if addressMatches && eventMatches {
			subscribers = append(subscribers, sub)
		}
	}
	return subscribers
}
