package result

import (
	"net/http"
	"time"
)

type Result[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Data      T      `json:"data,omitzero"`
}

func newResult[T any](code int, message string, data T) *Result[T] {
	return &Result[T]{
		Code:      code,
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
		Data:      data,
	}
}

func Success() *Result[any] {
	return newResult[any](http.StatusOK, "ok", nil)
}

func SuccessData[T any](data T) *Result[T] {
	return newResult[T](http.StatusOK, "ok", data)
}

func Err(code int, message string) *Result[any] {
	return newResult[any](code, message, nil)
}

func ErrBadRequest(message string) *Result[any] {
	return newResult[any](http.StatusBadRequest, message, nil)
}

func ErrInternal(message string) *Result[any] {
	return newResult[any](http.StatusInternalServerError, message, nil)
}

func ErrAuth(message string) *Result[any] {
	return newResult[any](http.StatusUnauthorized, message, nil)
}
