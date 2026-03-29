package route

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/biz/handler"
	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/pkg/httpx"
)

func RegisterRouter(router chi.Router) {
	router.Get("/health", httpx.Wrap(handler.Health))

	router.Route("/workflows", func(r chi.Router) {
		r.Get("/", httpx.Wrap(handler.ListWorkflows))
		r.Post("/", httpx.Wrap(handler.SaveWorkflow))
		r.Get("/{id}", httpx.Wrap(handler.GetWorkflow))
		r.Delete("/{id}", httpx.Wrap(handler.DeleteWorkflow))
	})

	router.Get("/plugins", httpx.Wrap(handler.ListPlugins))
	router.Get("/monitor", httpx.Wrap(handler.GetMonitorStats))

	router.Route("/sessions", func(r chi.Router) {
		r.Post("/", httpx.Wrap(handler.CreateWorkflowSession))
		r.Get("/health/{id}", httpx.Wrap(handler.WorkflowSessionHealth))
		r.Post("/renew/{id}", httpx.Wrap(handler.RenewWorkflowSession))
		r.Delete("/{id}", httpx.Wrap(handler.DeleteWorkflowSession))
	})

	router.Get("/realtime/{id}", httpx.Wrap(realtime.Connect))

	router.Handle("/*", http.HandlerFunc(handler.WebHandler))
}
