package transport

import (
	"encoding/binary"
	"fmt"
	"io"
)

// SendMessage sends a length-prefixed message over a TCP connection.
// Wire format: [ 4-byte big-endian length ][ payload bytes ]
func (c *CommClient) SendMessage(data []byte) error {
	if len(data) > PORT_READ_BUFFER_SIZE {
		return fmt.Errorf("message size %d exceeds 4-byte header limit of %d", len(data), PORT_READ_BUFFER_SIZE)
	}

	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(data)))

	// Write header + payload in one call to avoid partial writes
	frame := append(header, data...)
	_, err := c.conn.Write(frame)
	return err
}

// RecvMessage reads one length-prefixed message from a TCP connection.
// Returns the raw payload bytes, or an error (including io.EOF on disconnect).
func (c *CommClient) RecvMessage() ([]byte, error) {
	// Read the length header
	header := make([]byte, LENGTH_HEADER_SIZE)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint16(header)
	if msgLen == 0 {
		return []byte{}, nil
	}

	// Read exactly msgLen bytes
	payload := make([]byte, msgLen)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return nil, fmt.Errorf("failed to read message body (%d bytes): %w", msgLen, err)
	}

	return payload, nil
}
