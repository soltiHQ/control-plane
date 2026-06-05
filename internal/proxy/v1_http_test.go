package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// httpProxyV1.ApplyTask must return the TaskId from ApplyTaskResponse.
// Without it CP cannot later DeleteTask or GetTaskStatus for this exact
// run, which breaks the uninstall (delete-on-target-removal) flow.
func TestHttpProxyV1_ApplyTask_ReturnsTaskID(t *testing.T) {
	want := "sub-my-slot-42"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasSuffix(r.URL.Path, "/api/v1/tasks") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		// Drain so we don't trip test server's internal assertions.
		_, _ = io.Copy(io.Discard, r.Body)

		body, err := protojson.Marshal(&taskv1.ApplyTaskResponse{TaskId: want})
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := &httpProxyV1{endpoint: srv.URL, client: srv.Client()}

	got, err := p.ApplyTask(context.Background(), TaskSubmission{
		Spec: &taskv1.CreateSpec{Slot: "my-slot"},
	})
	if err != nil {
		t.Fatalf("ApplyTask: %v", err)
	}
	if got != want {
		t.Errorf("task id: got %q, want %q", got, want)
	}
}

// An empty task_id is a protocol violation — the agent accepted the
// apply but gave us no handle to manage the run. Fail loudly rather
// than pretend-it-is-synced.
func TestHttpProxyV1_ApplyTask_RejectsEmptyTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := protojson.Marshal(&taskv1.ApplyTaskResponse{TaskId: ""})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := &httpProxyV1{endpoint: srv.URL, client: srv.Client()}

	_, err := p.ApplyTask(context.Background(), TaskSubmission{Spec: &taskv1.CreateSpec{Slot: "s"}})
	if err == nil {
		t.Fatal("expected error for empty task_id")
	}
	if !errors.Is(err, ErrApplyTask) {
		t.Errorf("error should wrap ErrApplyTask, got %v", err)
	}
	if !strings.Contains(err.Error(), "empty task id") {
		t.Errorf("error should mention empty task id, got %q", err.Error())
	}
}

// When the agent rejects the spec (400 with SDK error envelope), the
// error surfaced by the proxy must contain both the label and message.
// Verified earlier via formatUnexpectedStatus; this test keeps the
// end-to-end wiring covered when the decoding helper is on the hot path.
func TestHttpProxyV1_ApplyTask_Surfaces400Envelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"InvalidRequest","message":"slot cannot be empty"}`))
	}))
	defer srv.Close()

	p := &httpProxyV1{endpoint: srv.URL, client: srv.Client()}

	_, err := p.ApplyTask(context.Background(), TaskSubmission{Spec: &taskv1.CreateSpec{Slot: ""}})
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !strings.Contains(err.Error(), "InvalidRequest") || !strings.Contains(err.Error(), "slot cannot be empty") {
		t.Errorf("error should carry SDK envelope, got %q", err.Error())
	}
}
