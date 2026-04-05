package transport

import (
	"fmt"
	"io"
	"net"

	packet "helios/generated/transport"
)

const (
	CHAN_BUFFER_SIZE      = 1 << 8   // Number of messages buffered in each channel
	PORT_READ_BUFFER_SIZE = 1 << 30  // Max message size (4-byte header limit is 4GB, limit to 1GB)
	LENGTH_HEADER_SIZE    = 4        // 4-byte header for length
)

type CommClient struct {
	//ContainerID string
	conn        net.Conn
	coreOutChan chan []byte //Outgoing messages from handler to HeliosCore; Write
	coreInChan  chan []byte //Incoming messages from HeliosCore to handler; Read
	contInChan  chan []byte //Incoming messages from container to handler;  Read
	recentPacket *packet.TransportPacket     //Placeholder for most recent packet
}

// Create a new communication client for a specific container
func NewCommClient(c net.Conn) *CommClient {
	client := CommClient{
		//ContainerID: containerID,
		conn:        c,
		coreOutChan: make(chan []byte, CHAN_BUFFER_SIZE),
		coreInChan:  make(chan []byte, CHAN_BUFFER_SIZE),
		contInChan:  make(chan []byte, CHAN_BUFFER_SIZE),
	}

	go client._listenForContainerMessages()
	go client._handleMessages()

	return &client
}

// Listen for incoming container messages and output to contInChan.
// Uses RecvMessage to read length-prefixed frames.
func (c *CommClient) _listenForContainerMessages() {
	for {
		msg, err := c.RecvMessage()
		if err != nil {
			if err == io.EOF {
				fmt.Printf("Container disconnected: %s\n", c.conn.RemoteAddr())
			} else {
				fmt.Printf("Error reading from container %s: %v\n", c.conn.RemoteAddr(), err)
			}
			return
		}

		if len(msg) > 0 {
			c.contInChan <- msg
		}
	}
}

// Handle messages from all channels in a continuous loop.
// Uses SendMessage to write length-prefixed frames.
func (c *CommClient) _handleMessages() {
	for {
		select {
		case msg := <-c.GetCoreInChan():
			// Process any incoming stuff from HeliosCore here
			if err := c.SendMessage(msg); err != nil {
				fmt.Printf("Error sending to container %s: %v\n", c.conn.RemoteAddr(), err)
				return
			}
		case msg := <-c.GetContInChan():
			c.GetCoreOutChan() <- msg // If we want to send it to Core

			packet, _ := UnmarshalTransportPacket(msg)
			c.recentPacket = packet

			fmt.Printf("Message received — id: %d, address: %s, data: %s, timestamp: %s\n",
					packet.Id,
					packet.Address,
					packet.Data,
					packet.Timestamp.AsTime(),
			)

			//go c._broadcast(msg)
			if err := c.SendMessage(msg); err != nil {
				fmt.Printf("Error sending to container %s: %v\n", c.conn.RemoteAddr(), err)
				return
			}
		}
	}
}

func (c *CommClient) _broadcast(msg []byte) {
	//TODO: Potentially move broadcast functionality to the core Helios
}
