package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestArmRequiresSuccessfulShutdownAndActuallyHoldsPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := occupied.Addr().String()
	defer occupied.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1.0/shutdown" {
			t.Error("wrong shutdown request")
		}
		occupied.Close()
		w.WriteHeader(204)
	}))
	defer server.Close()
	f := newFixture("hold", "123-1", "bible-ready-target--fault")
	if f.ready() {
		t.Fatal("unarmed helper admitted candidate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.arm(ctx, server.URL, address); err != nil {
		t.Fatal(err)
	}
	defer f.release()
	if !f.ready() {
		t.Fatal("armed helper must leave Bible readiness as the only gate")
	}
	other, err := net.Listen("tcp", address)
	if err == nil {
		other.Close()
		t.Fatal("port was not held")
	}
	f.release()
	other, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatal("port not released", err)
	}
	other.Close()
	if !f.ready() {
		t.Fatal("release must not break helper readiness")
	}
}

func TestDeniedShutdownNeverPretendsArmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	f := newFixture("hold", "123-1", "bible-ready-target--fault")
	if err := f.arm(context.Background(), server.URL, "127.0.0.1:0"); err == nil {
		t.Fatal("denied shutdown accepted")
	}
	if f.ready() {
		t.Fatal("denied capability admitted candidate")
	}
}

func TestArmWaitsForSidecarStartupWithinDeadline(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	f := newFixture("hold", "123-1", "bible-ready-target--fault")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.arm(ctx, server.URL, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer f.release()
	if calls != 2 || !f.ready() {
		t.Fatal("startup retry did not establish actual hold")
	}
}

func TestControlsAreLocalAndMethodRestricted(t *testing.T) {
	f := newFixture("idle", "123-1", "bible-ready-target--old")
	for _, tc := range []struct {
		method, path, remote string
		want                 int
	}{
		{"GET", "/ready", "10.0.0.1:12", 200},
		{"POST", "/release", "10.0.0.1:12", 403},
		{"GET", "/release", "127.0.0.1:12", 405},
		{"GET", "/state", "10.0.0.1:12", 403},
		{"POST", "/phase/evil", "127.0.0.1:12", 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.RemoteAddr = tc.remote
		w := httptest.NewRecorder()
		f.handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s=%d want%d", tc.method, tc.path, w.Code, tc.want)
		}
	}
}

func TestCallerSendsUniqueNonceAndRejectsBadJSON(t *testing.T) {
	bad := false
	nonces := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := r.URL.Query().Get("isolation_nonce")
		if nonce == "" || nonces[nonce] {
			t.Error("missing/duplicate nonce")
		}
		nonces[nonce] = true
		if bad {
			w.Write([]byte(`{"not":"versions"}`))
			return
		}
		w.Write([]byte(`[{"id":1,"code":"KJV","name":"Isolation fixture","updated_at":1,"locale":"en"}]`))
	}))
	defer server.Close()
	f := newFixture("caller", "123-1", "bible-ready-caller--baseline")
	good := f.sample(server.URL, "baseline", 2)
	if good.Status != 200 || !good.Valid || good.Nonce == "" || good.Hash == "" {
		t.Fatalf("bad sample %+v", good)
	}
	bad = true
	invalid := f.sample(server.URL, "baseline", 2)
	if invalid.Valid {
		t.Fatal("malformed versions response accepted")
	}
}

func TestPhaseAcknowledgementWaitsForPreviousRequest(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte(`[{"id":1,"code":"KJV","name":"Isolation fixture","updated_at":1}]`))
	}))
	defer server.Close()
	f := newFixture("caller", "123-1", "bible-ready-caller--baseline")
	f.phase = "baseline"
	done := make(chan struct{})
	go func() { f.recordSample(server.URL, 1); close(done) }()
	<-started
	acknowledged := make(chan struct{})
	go func() {
		r := httptest.NewRequest("POST", "/phase/transition", nil)
		r.RemoteAddr = "127.0.0.1:12"
		f.handler().ServeHTTP(httptest.NewRecorder(), r)
		close(acknowledged)
	}()
	select {
	case <-acknowledged:
		t.Fatal("phase acknowledged before prior request completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-done
	<-acknowledged
	if len(f.samples) != 1 || f.samples[0].Phase != "baseline" || f.phase != "transition" {
		t.Fatal("phase attribution corrupted")
	}
}
