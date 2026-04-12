package server

import (
	"context"
	"errors"
	"net"

	"helios/internal/logger"
)

type Server struct {
	listener net.Listener
}

func StartServer(ctx context.Context, addr string) (*Server, error) {
	logger.Infow("Starting server", "address", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
	}

	go server.listenForConnections(ctx)

	logger.Info("Server started and listening for connections")

	return server, nil
}

func (s *Server) listenForConnections(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) || isPeerOrLocalClose(err) {
				return
			}
			logger.Errorw("Error accepting connection", "error", err)
			continue
		}

		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	logger.Infow("New connection", "remote_address", conn.RemoteAddr())

	handler := NewConnectionHandler(ctx, conn)
	handler.Handle()
}

func (s *Server) Close() error {
	return s.listener.Close()
}
