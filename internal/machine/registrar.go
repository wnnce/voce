package machine

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/lxzan/gws"
)

type Registrar struct {
	gws.BuiltinEventHandler
	ctx         context.Context
	id          string
	gatewayAddr string
	port        int
}

func NewRegistrar(ctx context.Context, id, gatewayAddr string, port int) *Registrar {
	if id == "" {
		id = uuid.New().String()
	}
	return &Registrar{
		ctx:         ctx,
		id:          id,
		gatewayAddr: gatewayAddr,
		port:        port,
	}
}

func (r *Registrar) Start() {
	go r.reconnectLoop()
}

func (r *Registrar) reconnectLoop() {
	const (
		initialBackoff = 500 * time.Millisecond
		maxBackoff     = 10 * time.Second
	)
	backoff := initialBackoff
	for {
		if r.ctx.Err() != nil {
			slog.Info("stopping registrar", "id", r.id)
			return
		}
		if err := r.register(); err != nil {
			slog.Error("gateway registration failed, retrying",
				"error", err,
				"id", r.id,
				"gateway", r.gatewayAddr,
				"backoff", backoff)

			select {
			case <-r.ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		backoff = initialBackoff
	}
}

func (r *Registrar) register() error {
	params := url.Values{}
	params.Set("id", r.id)
	params.Set("port", fmt.Sprintf("%d", r.port))

	u := url.URL{
		Scheme:   "ws",
		Host:     r.gatewayAddr,
		Path:     "/register",
		RawQuery: params.Encode(),
	}

	socket, _, err := gws.NewClient(r, &gws.ClientOption{
		Addr: u.String(),
	})
	if err != nil {
		return err
	}
	socket.ReadLoop()
	return nil
}

func (r *Registrar) OnOpen(c *gws.Conn) {
	slog.Info("connected to gateway control plane", "id", r.id)
}

func (r *Registrar) OnPing(c *gws.Conn, payload []byte) {
	if err := c.WritePong(nil); err != nil {
		slog.Error("write pong to gateway failed", "error", err)
	}
}

func (r *Registrar) OnClose(c *gws.Conn, err error) {
	slog.Warn("gateway control connection closed", "id", r.id, "error", err)
}
