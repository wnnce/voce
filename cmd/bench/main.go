package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/lxzan/gws"
	"github.com/wnnce/voce/internal/protocol"
)

var (
	users        = flag.Int("u", 10, "Number of concurrent users")
	duration     = flag.Duration("d", 20*time.Second, "Test duration")
	interval     = flag.Duration("i", 50*time.Millisecond, "Send interval")
	target       = flag.String("t", "http://127.0.0.1:7001", "Target server base URL")
	workflowName = flag.String("w", "benchmark", "Workflow name")
	bucketsCount = flag.Int("b", 5, "Number of staggered buckets")
)

type LatencyStats struct {
	latencies []int64
	mu        sync.Mutex
	count     atomic.Int64
}

func (s *LatencyStats) Record(l int64) {
	s.mu.Lock()
	s.latencies = append(s.latencies, l)
	s.mu.Unlock()
	s.count.Add(1)
}

func (s *LatencyStats) Print() {
	if len(s.latencies) == 0 {
		fmt.Println("\n❌ No data collected.")
		return
	}
	sort.Slice(s.latencies, func(i, j int) bool { return s.latencies[i] < s.latencies[j] })
	count := len(s.latencies)
	p95, p99 := s.latencies[int(float64(count)*0.95)], s.latencies[int(float64(count)*0.99)]
	var sum int64
	for _, v := range s.latencies {
		sum += v
	}

	fmt.Printf("\n===== Voce Performance Report =====\n")
	fmt.Printf("Users/Buckets: %d / %d\n", *users, *bucketsCount)
	fmt.Printf("Packets:       %d\n", count)
	fmt.Printf("Avg RTT:       %d ms\n", sum/int64(count))
	fmt.Printf("P95 RTT:       %d ms\n", p95)
	fmt.Printf("P99 RTT:       %d ms\n", p99)
	fmt.Printf("Min/Max:       %d/%d ms\n", s.latencies[0], s.latencies[count-1])
	fmt.Printf("===================================\n")
}

type Client struct {
	gws.BuiltinEventHandler
	id      int
	socket  *gws.Conn
	stats   *LatencyStats
	payload []byte
	sentAt  [1024]atomic.Int64
}

func (c *Client) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	p := protocol.AcquirePacket()
	defer protocol.ReleasePacket(p)

	if err := p.Unmarshal(message.Bytes()); err != nil || p.Type != protocol.TypeAudio {
		return
	}

	if len(p.Payload) >= 8 {
		ts := int64(binary.BigEndian.Uint64(p.Payload[:8]))
		idx := (ts / interval.Milliseconds()) % 1024
		start := c.sentAt[idx].Swap(0)
		if start > 0 {
			c.stats.Record(time.Now().UnixMilli() - start)
		}
	}
}

func (c *Client) Send(now time.Time) {
	ts := now.UnixMilli()
	idx := (ts / interval.Milliseconds()) % 1024
	c.sentAt[idx].Store(ts)

	p := protocol.AcquirePacket()
	defer protocol.ReleasePacket(p)
	p.Type = protocol.TypeAudio

	binary.BigEndian.PutUint64(c.payload[:8], uint64(ts))
	p.SetPayload(c.payload)
	_ = c.socket.Writev(gws.OpcodeBinary, p.Header(), p.Payload)
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	stats := &LatencyStats{}
	clients := make([]*Client, 0, *users)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	fmt.Printf("🚀 Staggered Benchmark: %d users in %d buckets, %v interval\n", *users, *bucketsCount, *interval)

	for i := 0; i < *users; i++ {
		c, err := setupClient(i, stats)
		if err != nil {
			log.Printf("Fail user %d: %v", i, err)
			continue
		}
		clients = append(clients, c)
		if i > 0 && i%100 == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Calculate offset between buckets (e.g., 50ms interval / 5 buckets = 10ms offset)
	offsetPerBucket := interval.Milliseconds() / int64(*bucketsCount)

	// Distribution logic: divide clients into buckets
	var wg sync.WaitGroup
	for b := 0; b < *bucketsCount; b++ {
		startIdx := b * len(clients) / (*bucketsCount)
		endIdx := (b + 1) * len(clients) / (*bucketsCount)
		bucketClients := clients[startIdx:endIdx]

		wg.Add(1)
		go func(idx int, subClients []*Client) {
			defer wg.Done()
			// Stagger start: 0, 10ms, 20ms...
			time.Sleep(time.Duration(int64(idx)*offsetPerBucket) * time.Millisecond)

			ticker := time.NewTicker(*interval)
			defer ticker.Stop()
			for {
				select {
				case t := <-ticker.C:
					for _, c := range subClients {
						c.Send(t)
					}
				case <-ctx.Done():
					return
				}
			}
		}(b, bucketClients)
	}

	wg.Wait()
	for _, c := range clients {
		_ = c.socket.WriteClose(1000, nil)
	}
	stats.Print()
}

func setupClient(id int, stats *LatencyStats) (*Client, error) {
	p, _ := sonic.Marshal(map[string]string{"name": *workflowName})
	resp, err := http.Post(*target+"/sessions", "application/json", bytes.NewBuffer(p))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Data struct {
			SessionID string `json:"session_id"`
		}
	}
	if err = sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	c := &Client{
		id:      id,
		stats:   stats,
		payload: make([]byte, 1600),
	}
	wsURL := strings.Replace(*target, "http", "ws", 1) + "/realtime/" + res.Data.SessionID
	socket, response, err := gws.NewClient(c, &gws.ClientOption{Addr: wsURL})
	defer response.Body.Close()
	if err != nil {
		return nil, err
	}
	c.socket = socket
	go socket.ReadLoop()

	return c, nil
}
