package kuscia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrCodeQueryJob is kuscia ErrorCode KusciaAPIErrQueryJob. KusciaAPI
// QueryJob answers it (HTTP 200, status.code 11202) with the Kubernetes
// "kusciajobs... not found" message when the KusciaJob does not exist.
const ErrCodeQueryJob int32 = 11202

// IsJobNotFound reports whether err is a KusciaAPI QueryJob "job not exists"
// error (the job was deleted from Kuscia or never created there).
func IsJobNotFound(err error) bool {
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != ErrCodeQueryJob {
		return false
	}
	return strings.Contains(strings.ToLower(ae.Message), "not found")
}

// WatchEventType is kusciaapi EventType.
type WatchEventType int32

// kusciaapi EventType values.
const (
	WatchEventAdded     WatchEventType = 0
	WatchEventModified  WatchEventType = 1
	WatchEventDeleted   WatchEventType = 2
	WatchEventError     WatchEventType = 3
	WatchEventHeartbeat WatchEventType = 4
)

var watchEventNames = map[string]WatchEventType{
	"ADDED": WatchEventAdded, "MODIFIED": WatchEventModified, "DELETED": WatchEventDeleted,
	"ERROR": WatchEventError, "HEARTBEAT": WatchEventHeartbeat,
}

// UnmarshalJSON accepts the numeric form written by the KusciaAPI HTTP watch
// handler (encoding/json of the protobuf struct) and the enum name form
// (protojson) for robustness.
func (t *WatchEventType) UnmarshalJSON(b []byte) error {
	var n int32
	if err := json.Unmarshal(b, &n); err == nil {
		*t = WatchEventType(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("kuscia: invalid watch event type %s", string(b))
	}
	v, ok := watchEventNames[strings.ToUpper(s)]
	if !ok {
		return fmt.Errorf("kuscia: unknown watch event type %q", s)
	}
	*t = v
	return nil
}

func (t WatchEventType) String() string {
	for k, v := range watchEventNames {
		if v == t {
			return k
		}
	}
	return fmt.Sprintf("EventType(%d)", int32(t))
}

// WatchJobObject is kusciaapi JobStatus.
type WatchJobObject struct {
	JobID  string               `json:"job_id"`
	Status *JobStatusDetailInfo `json:"status,omitempty"`
}

// WatchJobEvent is kusciaapi WatchJobEventResponse.
type WatchJobEvent struct {
	Type   WatchEventType  `json:"type"`
	Object *WatchJobObject `json:"object,omitempty"`
}

// WatchJobPath is the KusciaAPI HTTP job watch endpoint
// (kuscia pkg/kusciaapi/bean/http_server_bean.go "job/watch").
const WatchJobPath = "/api/v1/job/watch"

// WatchJobs opens the KusciaAPI HTTP job watch stream (POST
// /api/v1/job/watch, a chunked response of back-to-back JSON
// WatchJobEventResponse objects plus a HEARTBEAT every 10s) and calls fn for
// every non-heartbeat event until the stream ends, ctx is cancelled or fn
// returns an error.
//
// timeoutSeconds is forwarded as WatchJobRequest.timeout_seconds (0 lets
// Kuscia use its maximum). A clean end of stream (server-side timeout)
// returns nil; an HTTP error (e.g. 404 on Kuscia versions without the HTTP
// watch) is returned as *APIError so callers can fall back to polling.
func (c *Client) WatchJobs(ctx context.Context, timeoutSeconds int64, fn func(*WatchJobEvent) error) error {
	if c.initErr != nil {
		return c.initErr
	}
	body, _ := json.Marshal(map[string]int64{"timeout_seconds": timeoutSeconds})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+WatchJobPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("kuscia: create watch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set(TokenHeader, c.token)
	}
	// The shared client has a whole-request timeout that would cut the
	// long-lived stream: reuse its transport without a timeout; ctx and the
	// server-side timeout_seconds bound the stream instead.
	hc := &http.Client{Transport: c.httpClient.Transport}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("kuscia: request %s: %w", WatchJobPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		ae := &APIError{Path: WatchJobPath, HTTPStatus: resp.StatusCode, Message: string(msg)}
		var env statusEnvelope
		if json.Unmarshal(msg, &env) == nil && env.Status != nil && env.Status.Code != 0 {
			ae.Code, ae.Message = env.Status.Code, env.Status.Message
		}
		return ae
	}
	dec := json.NewDecoder(resp.Body)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kuscia: decode watch stream: %w", err)
		}
		// A JSON body with a status envelope instead of events means the
		// server rejected the request (auth, validation).
		var env statusEnvelope
		if json.Unmarshal(raw, &env) == nil && env.Status != nil && env.Status.Code != 0 {
			return &APIError{Path: WatchJobPath, HTTPStatus: resp.StatusCode, Code: env.Status.Code, Message: env.Status.Message}
		}
		var ev WatchJobEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			return fmt.Errorf("kuscia: decode watch event: %w", err)
		}
		if ev.Type == WatchEventHeartbeat {
			continue
		}
		if err := fn(&ev); err != nil {
			return err
		}
	}
}
