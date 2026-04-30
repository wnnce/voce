package handler

import (
	"net/http"
	"slices"

	"github.com/invopop/jsonschema"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type PluginHandler struct {
}

func NewPluginHandler() *PluginHandler {
	return &PluginHandler{}
}

type PluginInfo struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitzero"`
	Schema      *jsonschema.Schema    `json:"schema,omitzero"`
	Inputs      []engine.Property     `json:"inputs,omitzero"`
	Outputs     []engine.Property     `json:"outputs,omitzero"`
	Ports       []engine.PortMetadata `json:"ports,omitzero"`
}

func (h *PluginHandler) ListPlugins(w http.ResponseWriter, r *http.Request) error {
	builders := engine.GetPluginBuilders()
	list := make([]PluginInfo, 0, len(builders))

	for _, b := range builders {
		list = append(list, PluginInfo{
			Name:        b.Name(),
			Description: b.Description(),
			Schema:      b.Schema(),
			Inputs:      b.Inputs(),
			Outputs:     b.Outputs(),
			Ports:       b.Ports(),
		})
	}

	slices.SortFunc(list, func(a, b PluginInfo) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return httpx.JSON(w, http.StatusOK, result.SuccessData(list))
}
