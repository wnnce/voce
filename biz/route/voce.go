package route

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	pb "github.com/wnnce/voce/api/voce/v1"
	"github.com/wnnce/voce/biz/handler"
	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type AppContainer struct {
	Workflow *handler.WorkflowHandler
	Session  *handler.SessionHandler
	Plugin   *handler.PluginHandler
	Monitor  *handler.MonitorHandler
	Machine  *handler.MachineHandler
	Realtime *realtime.Handler
	Grpc     pb.VoceServiceServer
}

func RegisterVoceRouter(router chi.Router, container AppContainer) {
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = httpx.JSON(w, http.StatusOK, result.Success())
	})

	router.Route("/workflows", func(r chi.Router) {
		r.Get("/", httpx.Wrap(container.Workflow.ListWorkflows))
		r.Post("/", httpx.Wrap(container.Workflow.SaveWorkflow))
		r.Get("/{id}", httpx.Wrap(container.Workflow.GetWorkflow))
		r.Delete("/{id}", httpx.Wrap(container.Workflow.DeleteWorkflow))
	})

	router.Get("/plugins", httpx.Wrap(container.Plugin.ListPlugins))
	router.Get("/monitor", httpx.Wrap(container.Monitor.GetMonitorStats))

	router.Route("/sessions", func(r chi.Router) {
		r.Post("/", httpx.Wrap(container.Session.CreateWorkflowSession))
		r.Get("/health/{id}", httpx.Wrap(container.Session.WorkflowSessionHealth))
		r.Post("/renew/{id}", httpx.Wrap(container.Session.RenewWorkflowSession))
		r.Delete("/{id}", httpx.Wrap(container.Session.DeleteWorkflowSession))
	})

	router.Get("/realtime/{id}", httpx.Wrap(container.Realtime.Connect))

	if container.Machine != nil {
		router.Get("/pool", httpx.Wrap(container.Machine.PoolConnection))
	}

	router.Handle("/*", http.HandlerFunc(handler.WebHandler))
}
