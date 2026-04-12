package server

import (
	"context"
	"errors"
	"fmt"
	"helios/generated/transport"
	"helios/internal/logger"
	"io"
	"net"
	"syscall"
)

const MAX_MAILBOX_SIZE = 128

type ClientInfo struct {
	version int
	address string
}

type ConnectionHandler struct {
	ctx     context.Context
	conn    net.Conn
	mailbox chan *transport.TransportMessage
}

func NewConnectionHandler(ctx context.Context, conn net.Conn) *ConnectionHandler {
	return &ConnectionHandler{
		ctx:     ctx,
		conn:    conn,
		mailbox: make(chan *transport.TransportMessage, MAX_MAILBOX_SIZE),
	}
}

func (h *ConnectionHandler) Handle() {
	clientInfo, err := h.performHandshake()

	if err != nil {
		if h.ctx.Err() != nil {
			return
		}

		if isPeerOrLocalClose(err) {
			logger.Debugw("Connection closed", "remote_address", h.conn.RemoteAddr(), "reason", err)
			return
		}

		logger.Errorw("Error reading from connection", "remote_address", h.conn.RemoteAddr(), "error", err)
	}

	logger.Debugw("Handshake successful", "remote_address", h.conn.RemoteAddr(), "client_info", clientInfo)

	go h.readLoop()
	go h.writeLoop()

	<-h.ctx.Done()
	logger.Infow("Closing connection", "remote_address", h.conn.RemoteAddr())
	h.Close()
}

func (h *ConnectionHandler) performHandshake() (ClientInfo, error) {
	// Start Handshake
	transportMessage, err := ReadMessage(h.conn)
	if err != nil {
		return ClientInfo{}, err
	}

	handshakeRequest := transportMessage.GetHandshakeRequest()
	if handshakeRequest == nil {
		return ClientInfo{}, fmt.Errorf("Expected handshake request, got: %v", transportMessage)
	}

	logger.Infow("Handshake request received", "remote_address", h.conn.RemoteAddr(), "message", handshakeRequest)

	clientVersion := int(handshakeRequest.GetVersion())
	if clientVersion != PROTOCOL_VERSION {
		return ClientInfo{}, fmt.Errorf("Unsupported protocol version from client: %d", handshakeRequest.GetVersion())
	}

	// Send Handshake Response
	handshakeResponse := &transport.TransportMessage{
		Message: &transport.TransportMessage_HandshakeResponse{
			HandshakeResponse: &transport.HandshakeResponse{
				Version: PROTOCOL_VERSION,
			},
		},
	}

	err = SendMessage(h.conn, handshakeResponse)
	if err != nil {
		return ClientInfo{}, err
	}

	logger.Infow("Handshake response sent", "remote_address", h.conn.RemoteAddr(), "message", handshakeResponse)

	clientInfo := ClientInfo{
		version: clientVersion,
		address: h.conn.RemoteAddr().String(),
	}

	return clientInfo, nil
}

func (h *ConnectionHandler) readLoop() {
	for {
		transportMessage, err := ReadMessage(h.conn)
		if err != nil {
			if h.ctx.Err() != nil {
				return
			}

			if isPeerOrLocalClose(err) {
				logger.Debugw("Connection closed", "remote_address", h.conn.RemoteAddr(), "reason", err)
				return
			}

			logger.Errorw("Error reading from connection", "remote_address", h.conn.RemoteAddr(), "error", err)
			continue
		}

		// Handle the message (for now we just log it)
		logger.Debugw("Message received from client", "remote_address", h.conn.RemoteAddr(), "message", transportMessage)
	}
}

func (h *ConnectionHandler) writeLoop() {
	for {
		select {
		case msg := <-h.mailbox:
			err := SendMessage(h.conn, msg)
			if err != nil {
				if h.ctx.Err() != nil {
					return
				}

				if isPeerOrLocalClose(err) {
					logger.Debugw("Connection closed", "remote_address", h.conn.RemoteAddr(), "reason", err)
					return
				}

				logger.Errorw("Error writing to connection", "remote_address", h.conn.RemoteAddr(), "error", err)
			}
		case <-h.ctx.Done():
			return
		}
	}
}

func (h *ConnectionHandler) Close() {
	h.conn.Close()
}

// isPeerOrLocalClose reports errors that usually mean the TCP session ended
// normally or abruptly on the other side, or we closed the socket.
func isPeerOrLocalClose(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return isPeerOrLocalClose(opErr.Err)
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ECONNRESET, syscall.EPIPE, syscall.ECONNABORTED, syscall.ENOTCONN:
			return true
		}
	}
	return false
}
