package recorder

import (
	"context"
	"fmt"
	"net"
	"time"

	vnc "github.com/amitbet/vnc2video"
)

type vncSession struct {
	conn          *vnc.ClientConn
	clientMessage chan vnc.ClientMessage
	serverMessage chan vnc.ServerMessage
	errorCh       chan error
}

func connectVNC(ctx context.Context, cfg Config) (*vncSession, error) {
	addr := net.JoinHostPort(cfg.VNCHost, fmt.Sprintf("%d", cfg.VNCPort))
	dialer := net.Dialer{Timeout: 5 * time.Second}
	nc, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	serverCh := make(chan vnc.ServerMessage, 128)
	clientCh := make(chan vnc.ClientMessage, 128)
	errCh := make(chan error, 16)

	security := []vnc.SecurityHandler{
		&vnc.ClientAuthNone{},
	}
	if cfg.VNCPassword != "" {
		security = append([]vnc.SecurityHandler{&vnc.ClientAuthVNC{Password: []byte(cfg.VNCPassword)}}, security...)
	}

	encodings := []vnc.Encoding{
		&vnc.RawEncoding{},
	}

	vncCfg := &vnc.ClientConfig{
		SecurityHandlers: security,
		DrawCursor:       true,
		PixelFormat:      vnc.PixelFormat32bit,
		ClientMessageCh:  clientCh,
		ServerMessageCh:  serverCh,
		Messages:         vnc.DefaultServerMessages,
		Encodings:        encodings,
		ErrorCh:          errCh,
	}

	conn, err := vnc.Connect(ctx, nc, vncCfg)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}

	for _, enc := range encodings {
		renderer, ok := enc.(vnc.Renderer)
		if ok {
			renderer.SetTargetImage(conn.Canvas)
		}
	}

	return &vncSession{
		conn:          conn,
		clientMessage: clientCh,
		serverMessage: serverCh,
		errorCh:       errCh,
	}, nil
}

func (s *vncSession) Close() {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.Close()
}
