package handler

import (
	"net/http"
	"runtime"
	"time"

	"github.com/wnnce/voce/biz/realtime"
	"github.com/wnnce/voce/internal/engine"
	"github.com/wnnce/voce/internal/schema"
	"github.com/wnnce/voce/pkg/httpx"
	"github.com/wnnce/voce/pkg/result"
)

type MonitorStats struct {
	// Go Runtime Metrics
	Goroutines   int    `json:"goroutines"`
	HeapAlloc    uint64 `json:"heap_alloc"`
	HeapIdle     uint64 `json:"heap_idle"`
	HeapInuse    uint64 `json:"heap_inuse"`
	StackInuse   uint64 `json:"stack_inuse"`
	NumGC        uint32 `json:"num_gc"`
	PauseTotalNs uint64 `json:"pause_total_ns"`
	LastGCTime   string `json:"last_gc_time"`
	SystemMem    uint64 `json:"system_mem"`

	// Business Metrics (Resource Pools)
	ActiveAudioCount    int64 `json:"active_audio_count"`
	ActiveSDVideoCount  int64 `json:"active_sd_video_count"`
	ActiveHDVideoCount  int64 `json:"active_hd_video_count"`
	ActiveFHDVideoCount int64 `json:"active_fhd_video_count"`

	// Connectivity & Throughput
	ActiveSessions    int64  `json:"active_sessions"`
	ActiveConnections int64  `json:"active_connections"`
	AudioTrafficIn    uint64 `json:"audio_traffic_in"`
	AudioTrafficOut   uint64 `json:"audio_traffic_out"`

	Timestamp int64 `json:"timestamp"`
}

func GetMonitorStats(w http.ResponseWriter, r *http.Request) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	conn, in, out := realtime.GetMonitorCounters()
	sess := int64(engine.DefaultSessionManager.Count()) // Direct call to global manager

	stats := &MonitorStats{
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
