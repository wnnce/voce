package httpx

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/wnnce/voce/internal/errcode"
	"github.com/wnnce/voce/pkg/result"
)

type WrapHandlerFunc func(w http.ResponseWriter, r *http.Request) error

func Wrap(h WrapHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		var srvErr *errcode.ServerError
		if errors.As(err, &srvErr) {
			_ = JSON(w, srvErr.Status, result.Err(srvErr.Code, srvErr.Message))
			return
		}
		_ = JSON(w, http.StatusInternalServerError, result.ErrInternal(err.Error()))
	}
}

func BindJSON(request *http.Request, target any) error {
	payload, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	return sonic.Unmarshal(payload, target)
}

func JSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return sonic.ConfigDefault.NewEncoder(w).Encode(value)
}

func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return "127.0.0.1"
	}

	return host
}
