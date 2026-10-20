// Test-owned localhost controls; this binary is never part of the Bible image.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var client = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type sample struct {
	Phase     string  `json:"phase"`
	Mono      float64 `json:"mono"`
	Nonce     string  `json:"nonce"`
	Status    int     `json:"status"`
	Version   int     `json:"version"`
	Valid     bool    `json:"valid"`
	Hash      string  `json:"hash"`
	ElapsedMS int64   `json:"elapsed_ms"`
	Failure   string  `json:"failure,omitempty"`
}
type observation struct {
	Phase    string  `json:"phase"`
	Mono     float64 `json:"mono"`
	Health   int     `json:"health"`
	Ready    int     `json:"ready"`
	Outbound int     `json:"outbound"`
	Held     bool    `json:"held"`
}
type fixture struct {
	flight                              sync.Mutex
	mu                                  sync.Mutex
	mode, run, revision, phase, problem string
	started                             time.Time
	armed                               bool
	held                                net.Listener
	sequence                            int
	samples                             []sample
	observations                        []observation
}

func newFixture(mode, run, revision string) *fixture {
	return &fixture{mode: mode, run: run, revision: revision, started: time.Now()}
}
func (f *fixture) ready() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mode != "hold" || f.armed
}
func (f *fixture) release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.held != nil {
		f.held.Close()
		f.held = nil
	}
}
func (f *fixture) arm(ctx context.Context, base, address string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1.0/shutdown", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 204 {
				break
			}
			if resp.StatusCode != 503 {
				return fmt.Errorf("shutdown capability denied status %d", resp.StatusCode)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("shutdown capability unavailable")
		case <-ticker.C:
		}
	}
	for {
		held, err := net.Listen("tcp", address)
		if err == nil {
			f.mu.Lock()
			f.held = held
			f.armed = true
			f.mu.Unlock()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("port hold capability unavailable")
		case <-ticker.C:
		}
	}
}
func status(url string) int {
	resp, err := client.Get(url)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
func (f *fixture) sample(base, phase string, version int) (s sample) {
	started := time.Now()
	defer func() { s.ElapsedMS = time.Since(started).Milliseconds() }()
	f.mu.Lock()
	f.sequence++
	nonce := "iso-" + f.run + "-" + strconv.Itoa(f.sequence)
	f.mu.Unlock()
	s = sample{Phase: phase, Mono: time.Since(f.started).Seconds(), Nonce: nonce, Version: version}
	path := fmt.Sprintf("/v1.0/invoke/bible-ready-target/method/api/bible/v%d/versions?isolation_nonce=%s", version, nonce)
	resp, err := client.Get(base + path)
	if err != nil {
		s.Failure = "transport"
		var network *net.OpError
		if errors.Is(err, context.DeadlineExceeded) {
			s.Failure = "timeout"
		} else if errors.As(err, &network) && network.Op == "dial" {
			s.Failure = "connect"
		}
		return s
	}
	defer resp.Body.Close()
	s.Status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(body) > 65536 {
		return s
	}
	hash := sha256.Sum256(body)
	s.Hash = hex.EncodeToString(hash[:])
	var rows []struct {
		ID        int    `json:"id"`
		Code      string `json:"code"`
		Name      string `json:"name"`
		Locale    string `json:"locale"`
		UpdatedAt int64  `json:"updated_at"`
	}
	if json.Unmarshal(body, &rows) != nil || len(rows) != 1 {
		return s
	}
	row := rows[0]
	s.Valid = row.ID > 0 && row.Code == "KJV" && row.Name == "Isolation fixture" && row.UpdatedAt > 0 && (version == 1 || row.Locale == "en")
	return s
}
func (f *fixture) recordSample(base string, version int) {
	f.flight.Lock()
	defer f.flight.Unlock()
	f.mu.Lock()
	phase := f.phase
	f.mu.Unlock()
	if phase == "" {
		return
	}
	s := f.sample(base, phase, version)
	f.mu.Lock()
	f.samples = append(f.samples, s)
	f.mu.Unlock()
}
func (f *fixture) monitor(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	version := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		f.mu.Lock()
		phase := f.phase
		full := len(f.samples) >= 15000 || len(f.observations) >= 10000
		held := f.held != nil
		f.mu.Unlock()
		if full {
			f.mu.Lock()
			f.problem = "evidence capacity exceeded"
			f.mu.Unlock()
			return
		}
		if f.mode == "caller" && phase != "" {
			f.recordSample("http://127.0.0.1:3500", version)
			version = 3 - version
		} else if f.mode == "hold" {
			o := observation{Phase: phase, Mono: time.Since(f.started).Seconds(), Held: held,
				Health: status("http://127.0.0.1:9999/health"), Ready: status("http://127.0.0.1:9999/ready"), Outbound: status("http://127.0.0.1:3500/v1.0/healthz/outbound")}
			f.mu.Lock()
			f.observations = append(f.observations, o)
			f.mu.Unlock()
		}
	}
}
func local(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback()
}
func (f *fixture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			if !f.ready() {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
			return
		}
		if !local(r) {
			w.WriteHeader(403)
			return
		}
		switch {
		case r.URL.Path == "/state":
			if r.Method != "GET" {
				w.WriteHeader(405)
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"revision": f.revision, "armed": f.armed, "held": f.held != nil,
				"problem": f.problem, "samples": f.samples, "observations": f.observations})
		case r.URL.Path == "/release":
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			f.release()
			w.WriteHeader(204)
		case strings.HasPrefix(r.URL.Path, "/phase/"):
			if r.Method != "POST" {
				w.WriteHeader(405)
				return
			}
			phase := strings.TrimPrefix(r.URL.Path, "/phase/")
			switch phase {
			case "baseline", "prepare_fault", "fault", "recover_transition", "recovered", "prepare_rollback_fault", "rollback_fault", "rollback_transition", "rollback", "stop":
			default:
				w.WriteHeader(400)
				return
			}
			f.flight.Lock()
			defer f.flight.Unlock()
			f.mu.Lock()
			if phase == "stop" {
				phase = ""
			}
			f.phase = phase
			f.mu.Unlock()
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	})
}
func control(action, nonce string) error {
	if !regexp.MustCompile(`^[a-z0-9]{16}$`).MatchString(nonce) {
		return fmt.Errorf("invalid nonce")
	}
	path, method := "/state", "GET"
	if action == "release" {
		path, method = "/release", "POST"
	} else if strings.HasPrefix(action, "phase-") {
		path, method = "/phase/"+strings.TrimPrefix(action, "phase-"), "POST"
	} else if action != "state" {
		return fmt.Errorf("invalid control")
	}
	req, _ := http.NewRequest(method, "http://127.0.0.1:8081"+path, nil)
	// Phase acknowledgement waits for the previous two-second sample to finish.
	controlClient := *client
	controlClient.Timeout = 5 * time.Second
	resp, err := controlClient.Do(req)
	if err != nil {
		return fmt.Errorf("control unavailable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 6_000_001))
	if err != nil || len(body) > 6_000_000 {
		return fmt.Errorf("invalid evidence size")
	}
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("control refused %d", resp.StatusCode)
	}
	if resp.StatusCode == 204 {
		body = []byte(`{"ok":true}`)
	}
	if !json.Valid(body) {
		return fmt.Errorf("invalid evidence JSON")
	}
	fmt.Printf("ISOLATION_%s=%s\n", nonce, base64.StdEncoding.EncodeToString(body))
	return nil
}
func main() {
	if len(os.Args) == 4 && os.Args[1] == "client" {
		if err := control(os.Args[2], os.Args[3]); err != nil {
			log.Fatal(err)
		}
		return
	}
	mode, run, revision := os.Getenv("FIXTURE_MODE"), os.Getenv("FIXTURE_RUN"), os.Getenv("CONTAINER_APP_REVISION")
	name := os.Getenv("CONTAINER_APP_NAME")
	if !regexp.MustCompile(`^[1-9][0-9]*-[1-9][0-9]*$`).MatchString(run) || !strings.HasPrefix(revision, name+"--") ||
		!((name == "bible-ready-target" && (mode == "idle" || mode == "hold")) || (name == "bible-ready-caller" && mode == "caller")) {
		log.Fatal("refusing non-isolated fixture context")
	}
	f := newFixture(mode, run, revision)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Minute)
	defer cancel()
	defer f.release()
	go f.monitor(ctx)
	if mode == "hold" {
		go func() {
			armCtx, done := context.WithTimeout(ctx, 30*time.Second)
			defer done()
			if err := f.arm(armCtx, "http://127.0.0.1:3500", "0.0.0.0:50002"); err != nil {
				f.mu.Lock()
				f.problem = err.Error()
				f.mu.Unlock()
			}
		}()
	}
	server := &http.Server{Addr: ":8081", Handler: f.handler(), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); server.Close() }()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
