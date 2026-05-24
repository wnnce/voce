package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/protocol"
)

// --- Configuration ---

type Config struct {
	Workflow    string        `json:"workflow"`
	Users       int           `json:"users"`
	Duration    time.Duration `json:"-"`
	RawDuration string        `json:"duration"`
	Interval    time.Duration `json:"-"`
	RawInterval string        `json:"interval"`
	Target      string        `json:"target"`
	Buckets     int           `json:"buckets"`
	SaveReport  bool          `json:"save_report"`
	EnablePprof bool          `json:"enable_pprof"`
	OutputDir   string        `json:"output_dir"`
}

func (c *Config) ValidateAndFill() {
	if c.Users <= 0 {
		c.Users = defaultConfig.Users
	}
	if c.Buckets <= 0 {
		c.Buckets = defaultConfig.Buckets
	}
	if c.Target == "" {
		c.Target = defaultConfig.Target
	}
	if c.Workflow == "" {
		c.Workflow = defaultConfig.Workflow
	}
	if d, err := time.ParseDuration(c.RawDuration); err == nil {
		c.Duration = d
	} else if c.Duration == 0 {
		c.Duration = defaultConfig.Duration
	}
	if i, err := time.ParseDuration(c.RawInterval); err == nil {
		c.Interval = i
	} else if c.Interval == 0 {
		c.Interval = defaultConfig.Interval
	}
}

var defaultConfig = Config{
	Workflow: "benchmark",
	Users:    10,
	Duration: 20 * time.Second,
	Interval: 50 * time.Millisecond,
	Target:   "http://127.0.0.1:7001",
	Buckets:  5,
}

// --- High Performance Histogram ---

const (
	maxLatencyMS = 60000 // 60s max tracking
)

type Histogram struct {
	buckets  [maxLatencyMS + 1]int64
	received atomic.Int64
	sent     atomic.Int64
	errors   atomic.Int64
	sum      atomic.Int64
}

func NewHistogram() *Histogram {
	return &Histogram{}
}

func (h *Histogram) Record(latencyMS int64) {
	if latencyMS < 0 {
		latencyMS = 0
	}
	if latencyMS > maxLatencyMS {
		latencyMS = maxLatencyMS
	}
	atomic.AddInt64(&h.buckets[latencyMS], 1)
	h.received.Add(1)
	h.sum.Add(latencyMS)
}

func (h *Histogram) Merge(other *Histogram) {
	sent := other.sent.Load()
	received := other.received.Load()
	if sent == 0 && received == 0 {
		return
	}
	for i := 0; i <= maxLatencyMS; i++ {
		val := atomic.LoadInt64(&other.buckets[i])
		if val > 0 {
			atomic.AddInt64(&h.buckets[i], val)
		}
	}
	h.received.Add(received)
	h.sent.Add(sent)
	h.errors.Add(other.errors.Load())
	h.sum.Add(other.sum.Load())
}

func (h *Histogram) ValueAtPercentile(p float64) int64 {
	received := h.received.Load()
	if received == 0 {
		return 0
	}
	target := int64(math.Ceil(p / 100.0 * float64(received)))
	if target < 1 {
		target = 1
	}
	var current int64
	for i := 0; i <= maxLatencyMS; i++ {
		current += atomic.LoadInt64(&h.buckets[i])
		if current >= target {
			return int64(i)
		}
	}
	return maxLatencyMS
}

// --- Data Structures ---

type BenchReport struct {
	Workflow    string  `json:"workflow"`
	TargetUsers int     `json:"target_users"`
	ActualUsers int     `json:"actual_users"`
	Buckets     int     `json:"buckets"`
	Duration    string  `json:"duration"`
	Sent        int64   `json:"sent_packets"`
	Received    int64   `json:"received_packets"`
	Errors      int64   `json:"send_errors"`
	LossRate    float64 `json:"loss_rate"`
	AvgRTT      int64   `json:"avg_rtt_ms"`
	P95RTT      int64   `json:"p95_rtt_ms"`
	P99RTT      int64   `json:"p99_rtt_ms"`
	MinRTT      int64   `json:"min_rtt_ms"`
	MaxRTT      int64   `json:"max_rtt_ms"`
	Timestamp   int64   `json:"timestamp"`
}

type Client struct {
	gws.BuiltinEventHandler
	id        int
	socket    *gws.Conn
	interval  time.Duration
	histogram *Histogram
	payload   []byte
	sentAt    [1024]atomic.Int64
}

type PprofCollector struct {
	cfg *Config
}

// --- Flags ---

var (
	u            = flag.Int("u", 10, "Number of concurrent users")
	d            = flag.Duration("d", 20*time.Second, "Test duration")
	i            = flag.Duration("i", 50*time.Millisecond, "Send interval")
	t            = flag.String("t", "http://127.0.0.1:7001", "Target server base URL")
	w            = flag.String("w", "benchmark", "Workflow name")
	b            = flag.Int("b", 5, "Number of staggered buckets")
	save         = flag.Bool("save", false, "Save report")
	pprof        = flag.Bool("pprof", false, "Enable auto pprof collection")
	planPath     = flag.String("plan", "", "Path to benchmark plan JSON file")
	outDir       = flag.String("o", "", "Output directory")
	cooldownTime = flag.Duration("cooldown", 40*time.Second, "Cooldown time between rounds")
)

// --- Main Lifecycle ---

func main() {
	flag.Parse()
	log.SetFlags(0)

	var rounds []Config
	if *planPath != "" {
		data, err := os.ReadFile(*planPath)
		if err != nil {
			log.Fatalf("Failed to read plan file: %v", err)
		}
		if err := sonic.Unmarshal(data, &rounds); err != nil {
			log.Fatalf("Failed to parse plan file: %v", err)
		}
	} else {
		rounds = []Config{{
			Workflow:    *w,
			Users:       *u,
			Duration:    *d,
			Interval:    *i,
			Target:      *t,
			Buckets:     *b,
			SaveReport:  *save,
			EnablePprof: *pprof,
			OutputDir:   *outDir,
		}}
	}

	for idx, cfg := range rounds {
		cfg.ValidateAndFill()
		fmt.Printf("\n▶️  Round %d/%d: %d users, %s duration, target: %s\n",
			idx+1, len(rounds), cfg.Users, cfg.Duration, cfg.Target)

		clients := bootstrap(&cfg)
		if len(clients) == 0 {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Duration)
		run(ctx, clients, &cfg)
		finalize(clients, &cfg)
		cancel()

		if idx < len(rounds)-1 {
			fmt.Printf("❄️  Cooling down for %v...\n", *cooldownTime)
			time.Sleep(*cooldownTime)
		}
	}
}

func bootstrap(cfg *Config) []*Client {
	clients := make([]*Client, 0, cfg.Users)
	startTime := time.Now()

	for i := 0; i < cfg.Users; i++ {
		c, err := newClient(i, cfg)
		if err != nil {
			fmt.Printf("\n❌ Fail user %d: %v", i, err)
			continue
		}
		clients = append(clients, c)

		// Update bootstrap progress bar
		percent := float64(len(clients)) / float64(cfg.Users) * 100
		info := fmt.Sprintf("Bootstrapping: %d / %d clients", len(clients), cfg.Users)
		renderBar(percent, info, "░", "█")

		if i > 0 && i%100 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	fmt.Printf("\n🚀 Bootstrap finished in %v. Actual users: %d\n", time.Since(startTime).Round(time.Millisecond), len(clients))
	return clients
}

func run(ctx context.Context, clients []*Client, cfg *Config) {
	if len(clients) == 0 {
		return
	}

	if cfg.EnablePprof {
		collector := &PprofCollector{cfg: cfg}
		collector.Collect(ctx)
	}

	// Start real-time progress bar
	go trackProgress(ctx, cfg.Duration)

	offsetPerBucket := cfg.Interval.Milliseconds() / int64(cfg.Buckets)
	var wg sync.WaitGroup

	for b := 0; b < cfg.Buckets; b++ {
		startIdx := b * len(clients) / cfg.Buckets
		endIdx := (b + 1) * len(clients) / cfg.Buckets
		bucketClients := clients[startIdx:endIdx]

		wg.Add(1)
		go func(idx int, subClients []*Client, interval time.Duration) {
			defer wg.Done()
			time.Sleep(time.Duration(int64(idx)*offsetPerBucket) * time.Millisecond)

			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					for _, c := range subClients {
						c.Send(interval)
					}
				case <-ctx.Done():
					return
				}
			}
		}(b, bucketClients, cfg.Interval)

		// Heartbeat (Ping) goroutine for this bucket
		go func(subClients []*Client) {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					for _, c := range subClients {
						c.Ping()
					}
				case <-ctx.Done():
					return
				}
			}
		}(bucketClients)
	}

	wg.Wait()
	fmt.Print("\n") // Lock the progress bar line

	// Wait for tail packets to arrive
	fmt.Printf("⏳ Waiting 5s for tail packets to arrive...\n")
	time.Sleep(5 * time.Second)

	// Send explicit Close packet to release sessions on the server
	fmt.Printf("🧹 Finalizing: Sending Close packets to %d clients...\n", len(clients))
	for _, c := range clients {
		c.Close()
	}

	// Final buffer for WS handshake and server-side cleanup
	time.Sleep(2 * time.Second)
}

func trackProgress(ctx context.Context, duration time.Duration) {
	start := time.Now()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final update to ensure 100% is displayed
			renderBar(100.0, fmt.Sprintf("Testing: %v / %v", duration, duration), " ", "▆")
			return
		case <-ticker.C:
			elapsed := time.Since(start)
			percent := float64(elapsed) / float64(duration) * 100
			if percent >= 100 {
				percent = 99.9 // Clamp to 99.9 until the final Done update
			}

			info := fmt.Sprintf("Testing: %v / %v", elapsed.Round(time.Second), duration)
			renderBar(percent, info, " ", "▆")
		}
	}
}

func renderBar(percent float64, info string, emptyChar, fillChar string) {
	const barLen = 30
	filled := int(percent / 100 * barLen)
	if filled > barLen {
		filled = barLen
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat(fillChar, filled) + strings.Repeat(emptyChar, barLen-filled)
	// Improved spacing: [%s]  %5.1f%%  |  %s
	fmt.Printf("\r[%s]  %5.1f%%  |  %s", bar, percent, info)
}

func finalize(clients []*Client, cfg *Config) {
	globalHdr := NewHistogram()
	for _, c := range clients {
		globalHdr.Merge(c.histogram)
	}

	report := printReport(globalHdr, len(clients), cfg)
	if cfg.SaveReport && globalHdr.received.Load() > 0 {
		exportJSON(report, cfg)
	}
}

// --- Client Implementation ---

func newClient(id int, cfg *Config) (*Client, error) {
	p, _ := sonic.Marshal(map[string]string{"name": cfg.Workflow})
	resp, err := http.Post(cfg.Target+"/sessions", "application/json", bytes.NewBuffer(p))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to create session, status: %d", resp.StatusCode)
	}

	var res struct {
		Data struct {
			SessionID string `json:"session_id"`
		}
	}
	if err = sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	c := &Client{
		id:        id,
		interval:  cfg.Interval,
		payload:   make([]byte, 1600),
		histogram: NewHistogram(),
	}
	wsURL := strings.Replace(cfg.Target, "http", "ws", 1) + "/realtime/" + res.Data.SessionID
	socket, response, err := gws.NewClient(c, &gws.ClientOption{Addr: wsURL})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	c.socket = socket
	go socket.ReadLoop()

	return c, nil
}

func (c *Client) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	p := protocol.AcquirePacket()
	defer protocol.ReleasePacket(p)

	if err := p.Unmarshal(message.Bytes()); err != nil || p.Type != protocol.TypeAudio {
		return
	}
	if len(p.Payload) < 8 {
		return
	}

	ts := int64(binary.BigEndian.Uint64(p.Payload[:8]))
	idx := (ts / c.interval.Milliseconds()) % 1024
	start := c.sentAt[idx].Swap(0)
	if start > 0 {
		c.histogram.Record(time.Now().UnixMilli() - start)
	}
}

func (c *Client) Send(interval time.Duration) {
	ts := time.Now().UnixMilli()
	idx := (ts / interval.Milliseconds()) % 1024
	c.sentAt[idx].Store(ts)
	c.histogram.sent.Add(1)

	p := protocol.AcquirePacket()
	defer protocol.ReleasePacket(p)
	p.Type = protocol.TypeAudio

	binary.BigEndian.PutUint64(c.payload[:8], uint64(ts))
	p.SetPayload(c.payload)
	if err := c.socket.Writev(gws.OpcodeBinary, p.Header(), p.Payload); err != nil {
		c.histogram.errors.Add(1)
	}
}

func (c *Client) Ping() {
	_ = c.socket.WritePing(nil)
}

func (c *Client) Close() {
	p := protocol.AcquirePacket()
	defer protocol.ReleasePacket(p)
	p.Type = protocol.TypeClose
	_ = c.socket.Writev(gws.OpcodeBinary, p.Header(), p.Payload)
}

// --- Reporting & Exporting ---

func printReport(h *Histogram, actualUsers int, cfg *Config) BenchReport {
	sent := h.sent.Load()
	received := h.received.Load()
	if sent == 0 {
		fmt.Println("\n❌ No data collected.")
		return BenchReport{}
	}

	avg := int64(0)
	if received > 0 {
		avg = h.sum.Load() / received
	}
	p95 := h.ValueAtPercentile(95)
	p99 := h.ValueAtPercentile(99)

	lost := sent - received
	lossRate := float64(lost) / float64(sent) * 100.0

	var minRTT, maxRTT int64 = -1, 0
	for i := 0; i <= maxLatencyMS; i++ {
		if atomic.LoadInt64(&h.buckets[i]) > 0 {
			if minRTT == -1 {
				minRTT = int64(i)
			}
			maxRTT = int64(i)
		}
	}
	if minRTT == -1 {
		minRTT = 0
	}

	fmt.Printf("\n===== Voce Performance Report =====\n")
	fmt.Printf("Workflow:      %s\n", cfg.Workflow)
	fmt.Printf("Target Users:  %d\n", cfg.Users)
	fmt.Printf("Actual Users:  %d\n", actualUsers)
	fmt.Printf("Buckets:       %d\n", cfg.Buckets)
	fmt.Printf("Sent:          %d\n", sent)
	fmt.Printf("Received:      %d\n", received)
	fmt.Printf("Errors:        %d\n", h.errors.Load())
	fmt.Printf("Lost:          %d\n", lost)
	fmt.Printf("Loss Rate:     %.2f%%\n", lossRate)
	fmt.Printf("Avg RTT:       %d ms\n", avg)
	fmt.Printf("P95 RTT:       %d ms\n", p95)
	fmt.Printf("P99 RTT:       %d ms\n", p99)
	fmt.Printf("Min/Max:       %d/%d ms\n", minRTT, maxRTT)
	fmt.Printf("===================================\n")

	return BenchReport{
		Workflow:    cfg.Workflow,
		TargetUsers: cfg.Users,
		ActualUsers: actualUsers,
		Buckets:     cfg.Buckets,
		Duration:    cfg.Duration.String(),
		Sent:        sent,
		Received:    received,
		Errors:      h.errors.Load(),
		LossRate:    lossRate,
		AvgRTT:      avg,
		P95RTT:      p95,
		P99RTT:      p99,
		MinRTT:      minRTT,
		MaxRTT:      maxRTT,
		Timestamp:   time.Now().Unix(),
	}
}

func exportJSON(report BenchReport, cfg *Config) {
	dir := cfg.OutputDir
	if dir == "" {
		dir = "."
	}
	_ = os.MkdirAll(dir, 0755)

	filename := filepath.Join(dir, fmt.Sprintf("%s_%d_%d_%s_%d.json",
		cfg.Workflow, cfg.Users, cfg.Buckets, cfg.Duration.String(), report.Timestamp))

	data, _ := sonic.MarshalIndent(report, "", "  ")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		log.Printf("Failed to export report: %v", err)
	} else {
		fmt.Printf("Report exported to %s\n", filename)
	}
}

// --- Pprof Collection Implementation ---

func (p *PprofCollector) Collect(ctx context.Context) {
	go p.captureSnapshots(ctx)
	go p.captureTrace(ctx)
	go p.captureCPU(ctx)
}

func (p *PprofCollector) captureSnapshots(ctx context.Context) {
	interval := p.cfg.Duration / 10
	for i := 1; i <= 10; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			p.download("heap", ".prof", i, "/debug/pprof/heap")
			p.download("block", ".prof", i, "/debug/pprof/block")
			p.download("mutex", ".prof", i, "/debug/pprof/mutex")
		}
	}
}

func (p *PprofCollector) captureTrace(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(p.cfg.Duration / 2):
		p.download("trace", ".out", 0, "/debug/pprof/trace?seconds=5")
	}
}

func (p *PprofCollector) captureCPU(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(p.cfg.Duration / 4):
		seconds := int(p.cfg.Duration.Seconds() / 3)
		if seconds > 30 {
			seconds = 30
		}
		if seconds < 1 {
			seconds = 1
		}
		path := fmt.Sprintf("/debug/pprof/profile?seconds=%d", seconds)
		p.download("profile", ".pprof", 0, path)
	}
}

func (p *PprofCollector) download(typ, ext string, index int, path string) {
	resp, err := http.Get(p.cfg.Target + path)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	dir := p.cfg.OutputDir
	if dir == "" {
		dir = "."
	}
	_ = os.MkdirAll(dir, 0755)

	name := fmt.Sprintf("%s_%s_%d_%d_%s", typ, p.cfg.Workflow, p.cfg.Users, p.cfg.Buckets, p.cfg.Duration.String())
	if index > 0 {
		name = fmt.Sprintf("%s_%02d", name, index)
	}

	out, err := os.Create(filepath.Join(dir, name+ext))
	if err != nil {
		return
	}
	defer out.Close()

	_, _ = io.Copy(out, resp.Body)
}
