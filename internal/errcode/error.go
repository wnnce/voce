package errcode

import "net/http"

type ServerError struct {
	Status  int    `json:"-"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *ServerError) Error() string {
	return s.Message
}

func New(status int, code int, message string) *ServerError {
	return &ServerError{
		Status:  status,
		Code:    code,
		Message: message,
	}
}

func NewBadRequest(msg string) *ServerError {
	return New(http.StatusBadRequest, http.StatusBadRequest, msg)
}

func NewInternal(msg string) *ServerError {
	return New(http.StatusInternalServerError, http.StatusInternalServerError, msg)
}
