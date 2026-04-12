package server

import (
	"encoding/binary"
	"fmt"
	"helios/generated/transport"
	"helios/internal/logger"
	"io"

	"google.golang.org/protobuf/proto"
)

const PROTOCOL_VERSION = 1

const MAX_FRAME_BYTES = 16 * 1024 * 1024 // 16MB

func ReadMessage(buffer io.Reader) (*transport.TransportMessage, error) {
	// Message is a 4-byte network byte order length prefix followed by the message
	var lengthPrefix [4]byte

	if _, err := io.ReadFull(buffer, lengthPrefix[:]); err != nil {
		return nil, err
	}

	n := binary.BigEndian.Uint32(lengthPrefix[:])

	if n == 0 {
		return nil, fmt.Errorf("Invalid message length: %d", n)
	}

	if n > MAX_FRAME_BYTES {
		return nil, fmt.Errorf("Message length exceeds max frame size: %d", n)
	}

	messageBytes := make([]byte, n)

	if _, err := io.ReadFull(buffer, messageBytes); err != nil {
		return nil, err
	}

	message := &transport.TransportMessage{}
	err := proto.Unmarshal(messageBytes, message)

	logger.Debugw("Message read from buffer", "length", n, "message", message)

	return message, err
}

func SendMessage(buffer io.Writer, message *transport.TransportMessage) error {
	messageBytes, err := proto.Marshal(message)
	if err != nil {
		return err
	}

	if len(messageBytes) == 0 {
		return fmt.Errorf("Invalid message length: %d", len(messageBytes))
	}

	if len(messageBytes) > MAX_FRAME_BYTES {
		return fmt.Errorf("Message size exceeds max frame size: %d", len(messageBytes))
	}

	err = binary.Write(buffer, binary.BigEndian, uint32(len(messageBytes)))
	if err != nil {
		return err
	}

	_, err = buffer.Write(messageBytes)
	return err
}
