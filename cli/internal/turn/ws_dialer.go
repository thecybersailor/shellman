package turn

import (
	"context"

	"github.com/coder/websocket"
)

type RealDialer struct{}

func (RealDialer) Dial(ctx context.Context, url string) (Socket, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	return &realSocket{conn: conn}, nil
}

type realSocket struct {
	conn *websocket.Conn
}

func (s *realSocket) ReadText(ctx context.Context) (string, error) {
	_, data, err := s.conn.Read(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *realSocket) WriteText(ctx context.Context, text string) error {
	return s.conn.Write(ctx, websocket.MessageText, []byte(text))
}

func (s *realSocket) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}
