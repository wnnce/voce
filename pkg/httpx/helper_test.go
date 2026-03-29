package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wnnce/voce/internal/errcode"
)

func TestJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"foo": "bar"}
	require.NoError(t, JSON(w, http.StatusOK, data))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var res map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&res))
	assert.Equal(t, "bar", res["foo"])
}

func TestBindJSON(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		body := `{"name":"voce"}`
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		var target struct {
			Name string `json:"name"`
		}
		require.NoError(t, BindJSON(req, &target))
		assert.Equal(t, "voce", target.Name)
	})

	t.Run("invalid json", func(t *testing.T) {
		body := `{"name":`
		req := httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		var target any
		assert.Error(t, BindJSON(req, &target))
	})
}

func TestWrap(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := func(w http.ResponseWriter, r *http.Request) error {
			return nil
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		Wrap(h)(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("server error mapping", func(t *testing.T) {
		h := func(w http.ResponseWriter, r *http.Request) error {
			return errcode.New(http.StatusBadRequest, 40001, "custom error")
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		Wrap(h)(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var res map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&res))
		assert.InDelta(t, 40001, res["code"], 0.1)
	})

	t.Run("generic error fallback", func(t *testing.T) {
		h := func(w http.ResponseWriter, r *http.Request) error {
			return errors.New("boom")
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		Wrap(h)(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
