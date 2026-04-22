package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/biz/types"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type MonitorHandler struct {
	sm *engine.SessionManager
}

func NewMonitorHandler(sm *engine.SessionManager) *MonitorHandler {
	return &MonitorHandler{
		sm: sm,
	}
}

func (h *MonitorHandler) GetMonitorStats(w http.ResponseWriter, r *http.Request) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	conn, in, out := realtime.GetMonitorCounters()
	sess := int64(h.sm.Count())

	stats := &types.MonitorStats{
		Goroutines:   runtime.NumGoroutine(),
		HeapAlloc:    m.HeapAlloc,
		HeapIdle:     m.HeapIdle,
		HeapInuse:    m.HeapInuse,
		StackInuse:   m.StackInuse,
		NumGC:        m.NumGC,
		PauseTotalNs: m.PauseTotalNs,
		LastGCTime:   time.Unix(0, int64(m.LastGC)).Format(time.RFC3339),
		SystemMem:    m.Sys,

		ActiveAudioCount:    schema.LoadActiveAudioCount(),
		ActiveSDVideoCount:  schema.LoadActiveSDVideoCount(),
		ActiveHDVideoCount:  schema.LoadActiveHDVideoCount(),
		ActiveFHDVideoCount: schema.LoadActiveFHDVideoCount(),

		ActiveSessions:    sess,
		ActiveConnections: conn,
		AudioTrafficIn:    in,
		AudioTrafficOut:   out,

		Timestamp: time.Now().UnixMilli(),
	}

	return httpx.JSON(w, http.StatusOK, result.SuccessData(stats))
}
