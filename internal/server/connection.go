package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"helios/generated/transport"
	"helios/internal/logger"
	"io"
	"net"
	"syscall"

	"google.golang.org/protobuf/proto"
)

const MAX_FRAME_BYTES = 16 * 1024 * 1024 // 16MB

type ConnectionHandler struct {
	conn net.Conn
}

func (h *ConnectionHandler) Handle(ctx context.Context) {
	// Start Handshake
	handshakeRequestMessage, err := h.getNextMessage(ctx)
	if err != nil {
		h.handleError(ctx, err)
		return
	}

	// Parse Handshake Request
	handshakeRequest := &transport.HandshakeRequest{}
	err = proto.Unmarshal(handshakeRequestMessage, handshakeRequest)
	if err != nil {
		h.handleError(ctx, err)
		return
	}

	logger.Infow("Handshake request received", "remote_address", h.conn.RemoteAddr(), "message", handshakeRequest)

	// Send Handshake Response
	handshakeResponse := &transport.HandshakeResponse{
		Version: 1,
	}
	handshakeResponseData, err := proto.Marshal(handshakeResponse)
	if err != nil {
		return
	}
	err = h.sendMessage(ctx, handshakeResponseData)
	if err != nil {
		return
	}

	logger.Infow("Handshake response sent", "remote_address", h.conn.RemoteAddr(), "message", handshakeResponse)

	// Start Event Loop
	for {
		message, err := h.getNextMessage(ctx)
		if err != nil {
			h.handleError(ctx, err)
			return
		}

		logger.Infow("Message received", "remote_address", h.conn.RemoteAddr(), "message", string(message))
	}
}

func (h *ConnectionHandler) getNextMessage(ctx context.Context) ([]byte, error) {
	// Message is a 4-byte network byte order length prefix followed by the message
	var lengthPrefix [4]byte
	if _, err := io.ReadFull(h.conn, lengthPrefix[:]); err != nil {
		h.handleError(ctx, err)
		return nil, err
	}

	n := binary.BigEndian.Uint32(lengthPrefix[:])
	if n == 0 {
		logger.Errorw("Invalid message length", "remote_address", h.conn.RemoteAddr(), "length", n)
		return nil, fmt.Errorf("Invalid message length: %d", n)
	}
	if n > MAX_FRAME_BYTES {
		logger.Errorw("Message length exceeds max frame size", "remote_address", h.conn.RemoteAddr(), "length", n, "max_size", MAX_FRAME_BYTES)
		return nil, fmt.Errorf("Message length exceeds max frame size: %d", n)
	}

	message := make([]byte, n)
	if _, err := io.ReadFull(h.conn, message); err != nil {
		h.handleError(ctx, err)
		return nil, err
	}
	return message, nil
}

func (h *ConnectionHandler) sendMessage(ctx context.Context, message []byte) error {
	// Check message size is within the allowed range
	if len(message) == 0 {
		logger.Errorw("Invalid message length", "remote_address", h.conn.RemoteAddr(), "length", len(message))
		return fmt.Errorf("Invalid message length: %d", len(message))
	}
	if len(message) > MAX_FRAME_BYTES {
		logger.Errorw("Message size exceeds max frame size", "remote_address", h.conn.RemoteAddr(), "size", len(message), "max_size", MAX_FRAME_BYTES)
		return fmt.Errorf("message size exceeds max frame size: %d", len(message))
	}

	// First send the length of the message in network byte order, then the message
	err := binary.Write(h.conn, binary.BigEndian, uint32(len(message)))
	if err != nil {
		return err
	}
	_, err = h.conn.Write(message)
	if err != nil {
		h.handleError(ctx, err)
		return err
	}
	return err
}

func (h *ConnectionHandler) handleError(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	if isPeerOrLocalClose(err) {
		logger.Debugw("connection closed", "remote_address", h.conn.RemoteAddr(), "reason", err)
		return
	}
	logger.Errorw("Error reading from connection", "remote_address", h.conn.RemoteAddr(), "error", err)
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
