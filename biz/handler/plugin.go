package handler

import (
	"net/http"

	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type PluginHandler struct {
	store *engine.PluginStore
}

func NewPluginHandler(store *engine.PluginStore) *PluginHandler {
	return &PluginHandler{
		store: store,
	}
}

type PluginInfo struct {
	Namespace   string                `json:"namespace"`
	Name        string                `json:"name"`
	Description string                `json:"description,omitzero"`
	Schema      *jsonschema.Schema    `json:"schema,omitzero"`
	Inputs      []engine.Property     `json:"inputs,omitzero"`
	Outputs     []engine.Property     `json:"outputs,omitzero"`
	Ports       []engine.PortMetadata `json:"ports,omitzero"`
}

func (h *PluginHandler) ListPlugins(w http.ResponseWriter, _ *http.Request) error {
	descriptors := h.store.ListPluginBuilders()
	list := make([]PluginInfo, 0, len(descriptors))

	for _, descriptor := range descriptors {
		b := descriptor.Builder
		list = append(list, PluginInfo{
			Namespace:   descriptor.Namespace,
			Name:        b.Name(),
			Description: b.Description(),
			Schema:      b.Schema(),
			Inputs:      b.Inputs(),
			Outputs:     b.Outputs(),
			Ports:       b.Ports(),
		})
	}

	return httpx.JSON(w, http.StatusOK, result.SuccessData(list))
}
