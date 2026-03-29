package handler

import (
	"net/http"

	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

func Health(w http.ResponseWriter, _ *http.Request) error {
	return httpx.JSON(w, http.StatusOK, result.Success())
}
