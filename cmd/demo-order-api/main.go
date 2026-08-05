// Command demo-order-api is the target system for the live demo stack: a small order
// service with a configurable database connection pool, Prometheus metrics and an admin
// endpoint that faultctl uses to inject and restore a configuration change.
//
// It is deliberately simple and self-contained. Its purpose is to make the reference
// scenario reproducible against a running service, not to be a realistic application.
//
// Usage:
//
//	demo-order-api serve [--addr :8081]
//	demo-order-api load  [--target http://localhost:8081] [--rps 20] [--concurrency 40]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// sdd:impl DLD-1075

// pool models a fixed-size database connection pool. Acquiring blocks until a
// connection frees or the acquisition times out — which is the mechanism the reference
// scenario is about.
type pool struct {
	mu       sync.Mutex
	size     int
	slots    chan struct{}
	inUse    atomic.Int64
	acquired atomic.Int64
	timeouts atomic.Int64
}

func newPool(size int) *pool {
	p := &pool{}
	p.resize(size)
	return p
}

func (p *pool) resize(size int) {
	if size < 1 {
		size = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = size
	p.slots = make(chan struct{}, size)
	for i := 0; i < size; i++ {
		p.slots <- struct{}{}
	}
	p.inUse.Store(0)
}

func (p *pool) current() (chan struct{}, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.slots, p.size
}

// acquire waits up to timeout for a connection.
func (p *pool) acquire(timeout time.Duration) bool {
	slots, _ := p.current()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-slots:
		p.inUse.Add(1)
		p.acquired.Add(1)
		return true
	case <-timer.C:
		p.timeouts.Add(1)
		return false
	}
}

func (p *pool) release(slots chan struct{}) {
	p.inUse.Add(-1)
	select {
	case slots <- struct{}{}:
	default: // the pool was resized underneath us; the old slot simply disappears
	}
}

// server holds the demo service's mutable configuration and counters.
type server struct {
	mu     sync.RWMutex
	config map[string]string
	pool   *pool

	requests   atomic.Int64
	errors     atomic.Int64
	latencySum atomic.Int64 // microseconds
	started    time.Time
	rng        *rand.Rand
	rngMu      sync.Mutex
}

func newServer() *server {
	poolSize := envInt("DB_POOL_SIZE", 20)
	s := &server{
		config: map[string]string{
			"DB_POOL_SIZE":           strconv.Itoa(poolSize),
			"RATE_LIMIT_QPS":         envOr("RATE_LIMIT_QPS", "500"),
			"FEATURE_FLAG_SAFE_MODE": envOr("FEATURE_FLAG_SAFE_MODE", "false"),
			"PAYMENT_URL":            envOr("PAYMENT_URL", "https://payments.internal:8443"),
		},
		pool:    newPool(poolSize),
		started: time.Now(),
		rng:     rand.New(rand.NewSource(1)),
	}
	return s
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /admin/config", s.getConfig)
	mux.HandleFunc("POST /admin/config", s.setConfig)
	mux.HandleFunc("POST /orders", s.createOrder)
	mux.HandleFunc("GET /orders", s.listOrders)
	return mux
}

func (s *server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "uptime_seconds": int(time.Since(s.started).Seconds()),
	})
}

func (s *server) getConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.config))
	for k, v := range s.config {
		out[k] = v
	}
	writeJSON(w, http.StatusOK, out)
}

// setConfig is the injection point faultctl drives. It is deliberately the only way to
// change the service's behaviour, so a demo change is always visible in one place.
func (s *server) setConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}

	s.mu.Lock()
	previous := s.config[req.Key]
	s.config[req.Key] = req.Value
	s.mu.Unlock()

	if req.Key == "DB_POOL_SIZE" {
		if n, err := strconv.Atoi(req.Value); err == nil {
			s.pool.resize(n)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"key": req.Key, "previous": previous, "current": req.Value,
	})
}

func (s *server) createOrder(w http.ResponseWriter, r *http.Request) { s.serveOrder(w, r) }
func (s *server) listOrders(w http.ResponseWriter, r *http.Request)  { s.serveOrder(w, r) }

// serveOrder simulates a request that needs a database connection. Under a reduced
// pool the acquisition times out, which is exactly the failure the reference scenario
// investigates.
func (s *server) serveOrder(w http.ResponseWriter, _ *http.Request) {
	start := time.Now()
	s.requests.Add(1)

	slots, _ := s.pool.current()
	if !s.pool.acquire(300 * time.Millisecond) {
		s.errors.Add(1)
		s.latencySum.Add(time.Since(start).Microseconds())
		http.Error(w, "db connection timeout after 300ms", http.StatusInternalServerError)
		return
	}
	defer s.pool.release(slots)

	// A short, jittered hold on the connection.
	s.rngMu.Lock()
	hold := time.Duration(8+s.rng.Intn(12)) * time.Millisecond
	s.rngMu.Unlock()
	time.Sleep(hold)

	s.latencySum.Add(time.Since(start).Microseconds())
	writeJSON(w, http.StatusOK, map[string]any{
		"order_id": fmt.Sprintf("ord-%d", s.requests.Load()),
		"status":   "accepted",
	})
}

// metrics renders the Prometheus text exposition format by hand: four series is not
// worth a client library (ADR-001).
func (s *server) metrics(w http.ResponseWriter, _ *http.Request) {
	requests := s.requests.Load()
	errors := s.errors.Load()
	_, size := s.pool.current()
	inUse := s.pool.inUse.Load()

	var errorRate, meanLatency float64
	if requests > 0 {
		errorRate = float64(errors) / float64(requests)
		meanLatency = float64(s.latencySum.Load()) / float64(requests) / 1000
	}
	saturation := 0.0
	if size > 0 {
		saturation = float64(inUse) / float64(size)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, `# HELP http_requests_total Total requests served.
# TYPE http_requests_total counter
http_requests_total{service="order-api"} %d
# HELP http_5xx_total Total 5xx responses.
# TYPE http_5xx_total counter
http_5xx_total{service="order-api"} %d
# HELP http_5xx_rate Share of requests answered with a 5xx.
# TYPE http_5xx_rate gauge
http_5xx_rate{service="order-api"} %.6f
# HELP http_request_duration_mean_ms Mean request duration in milliseconds.
# TYPE http_request_duration_mean_ms gauge
http_request_duration_mean_ms{service="order-api"} %.3f
# HELP db_pool_size Configured database connection pool size.
# TYPE db_pool_size gauge
db_pool_size{service="order-api"} %d
# HELP db_pool_in_use Connections currently held.
# TYPE db_pool_in_use gauge
db_pool_in_use{service="order-api"} %d
# HELP db_pool_saturation Connections held over pool size.
# TYPE db_pool_saturation gauge
db_pool_saturation{service="order-api"} %.6f
# HELP db_pool_timeouts_total Acquisitions that timed out.
# TYPE db_pool_timeouts_total counter
db_pool_timeouts_total{service="order-api"} %d
# HELP db_up Database availability.
# TYPE db_up gauge
db_up{service="order-api"} 1
`, requests, errors, errorRate, meanLatency, size, inUse, saturation, s.pool.timeouts.Load())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "serve":
		serve(os.Args[min(2, len(os.Args)):])
	case "load":
		load(os.Args[min(2, len(os.Args)):])
	default:
		fmt.Fprintf(os.Stderr, "demo-order-api: unknown command %q; use serve or load\n", command)
		os.Exit(2)
	}
}

func serve(args []string) {
	addr := flagValue(args, "--addr", envOr("ORDER_API_ADDR", ":8081"))
	s := newServer()
	srv := &http.Server{
		Addr: addr, Handler: s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Printf("demo order-api listening on %s (DB_POOL_SIZE=%s)\n", addr, s.config["DB_POOL_SIZE"])
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "demo-order-api: %v\n", err)
		os.Exit(1)
	}
}

// load drives steady traffic at the demo service, so the pool is genuinely contended.
func load(args []string) {
	target := flagValue(args, "--target", "http://localhost:8081")
	rps := atoiOr(flagValue(args, "--rps", "20"), 20)
	concurrency := atoiOr(flagValue(args, "--concurrency", "40"), 40)

	fmt.Printf("driving %d rps at %s with %d workers\n", rps, target, concurrency)
	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(time.Second / time.Duration(max(rps, 1)))
	defer ticker.Stop()

	sem := make(chan struct{}, concurrency)
	for range ticker.C {
		select {
		case sem <- struct{}{}:
		default:
			continue // already at the concurrency ceiling; shed rather than queue
		}
		go func() {
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost,
				strings.TrimRight(target, "/")+"/orders", strings.NewReader(`{"sku":"demo"}`))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := client.Do(req)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Close = true
			_ = res.Body.Close()
		}()
	}
}

func flagValue(args []string, name, def string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"=")
		}
	}
	return def
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
