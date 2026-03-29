package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

func ListWorkflows(w http.ResponseWriter, r *http.Request) error {
	list, err := engine.DefaultWorkflowManager.List()
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(list))
}

func GetWorkflow(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	cfg, err := engine.DefaultWorkflowManager.Get(id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(cfg))
}

func SaveWorkflow(w http.ResponseWriter, r *http.Request) error {
	var cfg engine.WorkflowConfig
	if err := httpx.BindJSON(r, &cfg); err != nil {
		return err
	}

	if err := engine.DefaultWorkflowManager.Save(cfg); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.SuccessData(cfg))
}

func DeleteWorkflow(w http.ResponseWriter, r *http.Request) error {
	id := chi.URLParam(r, "id")
	if err := engine.DefaultWorkflowManager.Delete(id); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, result.Success())
}
