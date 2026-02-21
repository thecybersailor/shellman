package turn

import (
	"context"
	"errors"
	"io"
)

type Socket interface {
	ReadText(ctx context.Context) (string, error)
	WriteText(ctx context.Context, text string) error
	Close() error
}

type WSClient struct {
	sock   Socket
	onText func(string)
}

type onTextSetter interface {
	SetOnText(func(string))
}

func NewWSClient(sock Socket) *WSClient {
	return &WSClient{sock: sock}
}

func (c *WSClient) OnText(fn func(string)) {
	c.onText = fn
	if s, ok := c.sock.(onTextSetter); ok {
		s.SetOnText(fn)
	}
}

func (c *WSClient) Run(ctx context.Context) error {
	for {
		text, err := c.sock.ReadText(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if c.onText != nil {
			c.onText(text)
		}
	}
}

func (c *WSClient) Send(ctx context.Context, text string) error {
	return c.sock.WriteText(ctx, text)
}

func (c *WSClient) Close() error {
	return c.sock.Close()
}

type FakeSocket struct {
	onText func(string)
	readCh chan string
}

func NewFakeSocket() *FakeSocket {
	return &FakeSocket{readCh: make(chan string, 8)}
}

func (f *FakeSocket) SetOnText(fn func(string)) {
	f.onText = fn
}

func (f *FakeSocket) EmitText(text string) {
	if f.onText != nil {
		f.onText(text)
		return
	}
	f.readCh <- text
}

func (f *FakeSocket) ReadText(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case text, ok := <-f.readCh:
		if !ok {
			return "", io.EOF
		}
		return text, nil
	}
}

func (f *FakeSocket) WriteText(ctx context.Context, text string) error {
	return nil
}

func (f *FakeSocket) Close() error {
	close(f.readCh)
	return nil
}
