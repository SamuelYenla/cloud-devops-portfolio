package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestWriteJSONSetsContentTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, discardLogger(), http.StatusCreated, map[string]string{"hello": "world"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body = %v", body)
	}
}

func TestErrorUsesAConsistentShape(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, discardLogger(), http.StatusBadRequest, "bad input")

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "bad input" {
		t.Errorf("error field = %q", body["error"])
	}
}

// Probes run every few seconds per pod; logging them would bury real traffic.
func TestLoggingSkipsProbesButRecordsRealRequests(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("GET /real", func(w http.ResponseWriter, r *http.Request) {})
	handler := Logging(log, mux)

	for _, path := range []string{"/healthz", "/readyz"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if buf.Len() != 0 {
		t.Errorf("probes were logged: %s", buf.String())
	}

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/real", nil))
	if !strings.Contains(buf.String(), `"path":"/real"`) {
		t.Errorf("real request was not logged: %s", buf.String())
	}
}

func TestLoggingRecordsTheActualStatus(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := Logging(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))

	if !strings.Contains(buf.String(), `"status":"418"`) {
		t.Errorf("status not captured: %s", buf.String())
	}
}

// The zero value must not be ready: a pod should not receive traffic before
// its listener is accepting.
func TestHealthZeroValueIsNotReady(t *testing.T) {
	var h Health
	if h.IsReady() {
		t.Error("zero-value Health reports ready")
	}
}

func TestReadinessFlipsButLivenessDoesNot(t *testing.T) {
	h := &Health{}
	mux := http.NewServeMux()
	h.Register(mux)

	status := func(path string) int {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}

	if got := status("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("readyz before ready = %d, want %d", got, http.StatusServiceUnavailable)
	}

	h.SetReady(true)
	if got := status("/readyz"); got != http.StatusOK {
		t.Errorf("readyz when ready = %d, want %d", got, http.StatusOK)
	}

	// Liveness must stay green during a drain, or the kubelet restarts a pod
	// that is deliberately shutting down.
	h.SetReady(false)
	if got := status("/readyz"); got != http.StatusServiceUnavailable {
		t.Errorf("readyz while draining = %d, want %d", got, http.StatusServiceUnavailable)
	}
	if got := status("/healthz"); got != http.StatusOK {
		t.Errorf("healthz while draining = %d, want %d", got, http.StatusOK)
	}
}

// The pod spec's terminationGracePeriodSeconds has to exceed these, or
// Kubernetes SIGKILLs the process mid-drain.
func TestGracePeriodBudgetLeavesHeadroom(t *testing.T) {
	const podGracePeriod = 30 // terminationGracePeriodSeconds in k8s/*/deployment.yaml

	needed := (DrainDelay + ShutdownTimeout).Seconds()
	if needed >= podGracePeriod {
		t.Errorf("drain %v + shutdown %v = %.0fs, which does not fit in the pod's %ds grace period",
			DrainDelay, ShutdownTimeout, needed, podGracePeriod)
	}
}
