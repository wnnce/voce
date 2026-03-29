package result

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSuccess(t *testing.T) {
	res := Success()
	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "ok", res.Message)
	assert.Nil(t, res.Data)
	assert.Positive(t, res.Timestamp)
	assert.LessOrEqual(t, res.Timestamp, time.Now().UnixMilli())
}

func TestSuccessData(t *testing.T) {
	data := map[string]string{"foo": "bar"}
	res := SuccessData(data)
	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "ok", res.Message)
	assert.Equal(t, data, res.Data)
}

func TestErr(t *testing.T) {
	code := http.StatusNotFound
	msg := "not found"
	res := Err(code, msg)
	assert.Equal(t, code, res.Code)
	assert.Equal(t, msg, res.Message)
	assert.Nil(t, res.Data)
}

func TestErrBadRequest(t *testing.T) {
	msg := "bad request error"
	res := ErrBadRequest(msg)
	assert.Equal(t, http.StatusBadRequest, res.Code)
	assert.Equal(t, msg, res.Message)
}

func TestErrInternal(t *testing.T) {
	msg := "internal server error"
	res := ErrInternal(msg)
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Equal(t, msg, res.Message)
}

func TestErrAuth(t *testing.T) {
	msg := "unauthorized"
	res := ErrAuth(msg)
	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Equal(t, msg, res.Message)
}

func TestResultGeneric(t *testing.T) {
	type User struct {
		Name string
	}
	user := User{Name: "Alice"}
	res := SuccessData(user)
	assert.Equal(t, "Alice", res.Data.Name)
}
