package llm

import (
	"context"
	"sync"

	"github.com/wnnce/voce/internal/plugins/base/llm/data"
)

type CacheSession struct {
	sessionId string
	messages  []data.Message
	mutex     sync.RWMutex
	capSize   int
}

func NewCacheSession(sessionId string, capSize int) *CacheSession {
	return &CacheSession{
		sessionId: sessionId,
		messages:  make([]data.Message, 0, capSize),
		capSize:   capSize,
	}
}

func (s *CacheSession) ID() string {
	return s.sessionId
}

func (s *CacheSession) AddMessage(_ context.Context, messages ...data.Message) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.messages = append(s.messages, messages...)
	if len(s.messages) > s.capSize {
		s.messages = s.messages[len(s.messages)-s.capSize:]
	}
}

func (s *CacheSession) Messages() []data.Message {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if len(s.messages) == 0 {
		return nil
	}

	result := make([]data.Message, len(s.messages))
	copy(result, s.messages)
	return result
}
