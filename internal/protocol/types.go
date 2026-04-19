package protocol

import "github.com/google/uuid"

const SessionKeySize = 16

type SessionKey [SessionKeySize]byte

func (k SessionKey) String() string {
	return uuid.UUID(k).String()
}

func ParseSessionKey(id string) (SessionKey, error) {
	var key SessionKey
	u, err := uuid.Parse(id)
	if err != nil {
		return key, err
	}
	copy(key[:], u[:])
	return key, nil
}

func NewSessionKey() SessionKey {
	return SessionKey(uuid.New())
}

type ConnectionState int32

const (
	ConnectionUnknown ConnectionState = iota
	ConnectionActive
	ConnectionConnecting
	ConnectionClosed
)
