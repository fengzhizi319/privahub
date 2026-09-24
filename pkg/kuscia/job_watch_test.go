package kuscia

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWatchJobs_StreamDecoding(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != WatchJobPath || r.Header.Get(TokenHeader) != "tok" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// Kuscia writes encoding/json of the protobuf structs back to back:
		// ADDED (type 0) is omitted by omitempty, heartbeats carry type 4.
		_, _ = io.WriteString(w, `{"type":4}`)
		_, _ = io.WriteString(w, `{"object":{"job_id":"j1","status":{"state":"Running","tasks":[{"task_id":"t1","state":"Running","progress":0.5}]}}}`)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, `{"type":2,"object":{"job_id":"j2","status":{"state":"Succeeded"}}}{"type":"MODIFIED","object":{"job_id":"j3"}}`)
	}))
	defer srv.Close()
	c := NewClient(&ClientConfig{Host: strings.TrimPrefix(srv.URL, "http://"), Protocol: "notls", Token: "tok"})
	var events []*WatchJobEvent
	if err := c.WatchJobs(context.Background(), 30, func(ev *WatchJobEvent) error {
		events = append(events, ev)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if gotBody["timeout_seconds"] != float64(30) {
		t.Fatalf("request body %v", gotBody)
	}
	if len(events) != 3 {
		t.Fatalf("events %d", len(events))
	}
	if events[0].Type != WatchEventAdded || events[0].Object.JobID != "j1" || events[0].Object.Status.Tasks[0].Progress != 0.5 {
		t.Fatalf("event0 %+v", events[0])
	}
	if events[1].Type != WatchEventDeleted || events[1].Object.JobID != "j2" {
		t.Fatalf("event1 %+v", events[1])
	}
	if events[2].Type != WatchEventModified || events[2].Type.String() != "MODIFIED" {
		t.Fatalf("event2 %+v", events[2])
	}
}

func TestWatchJobs_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get(TokenHeader) {
		case "old":
			http.NotFound(w, r)
		case "denied":
			_, _ = io.WriteString(w, `{"status":{"code":11100,"message":"unauthorized"}}`)
		default:
			_, _ = io.WriteString(w, `{"object":{"job_id":"j1"}}`)
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	noop := func(*WatchJobEvent) error { return nil }

	err := NewClient(&ClientConfig{Host: host, Token: "old"}).WatchJobs(context.Background(), 0, noop)
	var ae *APIError
	if !errors.As(err, &ae) || ae.HTTPStatus != http.StatusNotFound {
		t.Fatalf("404 err %v", err)
	}
	err = NewClient(&ClientConfig{Host: host, Token: "denied"}).WatchJobs(context.Background(), 0, noop)
	if !errors.As(err, &ae) || ae.Code != 11100 {
		t.Fatalf("status err %v", err)
	}
	stop := errors.New("stop")
	err = NewClient(&ClientConfig{Host: host}).WatchJobs(context.Background(), 0, func(*WatchJobEvent) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatalf("callback err %v", err)
	}
}

func TestIsJobNotFound(t *testing.T) {
	if !IsJobNotFound(&APIError{Code: ErrCodeQueryJob, Message: `kusciajobs.kuscia.secretflow "j1" not found`}) {
		t.Fatal("expected not found")
	}
	if IsJobNotFound(&APIError{Code: ErrCodeQueryJob, Message: "connection refused"}) {
		t.Fatal("other query errors are not not-found")
	}
	if IsJobNotFound(errors.New("not found")) || IsJobNotFound(nil) {
		t.Fatal("plain errors are not not-found")
	}
}
