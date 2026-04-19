package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/pkg/httpx"
)

// NewRouter initializes and returns the primary gateway router.
func NewRouter(h *GatewayHandler) http.Handler {
	r := chi.NewRouter()

	r.Get("/health", httpx.Wrap(h.ProxyToAny))
	r.Get("/plugins", httpx.Wrap(h.ProxyToAny))
	r.Get("/monitor", httpx.Wrap(h.HandleMonitorAggregate))

	r.Route("/workflows", func(r chi.Router) {
		r.Get("/", httpx.Wrap(h.ProxyToAny))
		r.Post("/", httpx.Wrap(h.ProxyToAny))
		r.Get("/{id}", httpx.Wrap(h.ProxyToAny))
		r.Delete("/{id}", httpx.Wrap(h.ProxyToAny))
	})

	r.Route("/sessions", func(r chi.Router) {
		r.Post("/", httpx.Wrap(h.HandleSessionCreate))
		r.Get("/health/{id}", httpx.Wrap(h.HandleSessionHealth))
		r.Post("/renew/{id}", httpx.Wrap(h.HandleSessionRenew))
		r.Delete("/{id}", httpx.Wrap(h.HandleSessionDelete))
	})

	r.Get("/register", httpx.Wrap(h.HandleRegister))

	r.Get("/realtime/{id}", httpx.Wrap(h.HandleRealtime))

	r.Handle("/*", http.HandlerFunc(h.WebHandler))

	return r
}
