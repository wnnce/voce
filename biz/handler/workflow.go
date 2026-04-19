package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type WorkflowHandler struct {
	wm engine.WorkflowConfigManager
}

func NewWorkflowHandler(wm engine.WorkflowConfigManager) *WorkflowHandler {
	return &WorkflowHandler{wm: wm}
}

func (h *WorkflowHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) error {
	list, err := h.wm.List()
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(list))
}

func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	cfg, err := h.wm.Get(id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(cfg))
}

func (h *WorkflowHandler) SaveWorkflow(w http.ResponseWriter, r *http.Request) error {
	var cfg engine.WorkflowConfig
	if err := httpx.BindJSON(r, &cfg); err != nil {
		return err
	}

	if err := h.wm.Save(cfg); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(cfg))
}

func (h *WorkflowHandler) DeleteWorkflow(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := h.wm.Delete(id); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.Success())
}
