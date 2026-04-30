package types

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
