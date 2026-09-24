package audit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// AnchorNow writes the current chain tail to the configured external anchors
// (append-only file and/or HTTP endpoint). The anchor is the trust root of
// the unkeyed chain: an attacker who rewrites the database cannot also
// rewrite a tail hash stored outside their reach.
func (s *Service) AnchorNow(ctx context.Context) error {
	if s.opt.AnchorFile == "" && s.opt.AnchorURL == "" {
		return nil
	}
	tail, err := s.store.Tail(ctx)
	if err != nil {
		return err
	}
	s.anchorMu.Lock()
	defer s.anchorMu.Unlock()
	if tail.Seq == s.lastAnchorSeq && tail.Hash == s.lastAnchorHash {
		return nil // unchanged
	}
	a := Anchor{Time: time.Now().UTC().Format(time.RFC3339Nano), Seq: tail.Seq, Hash: tail.Hash,
		Node: s.opt.NodeID, Algorithm: HashAlgorithm}
	var firstErr error
	if s.opt.AnchorFile != "" {
		if err := appendAnchorFile(s.opt.AnchorFile, a); err != nil {
			metricAnchors.WithLabelValues("file", "error").Inc()
			firstErr = fmt.Errorf("anchor file: %w", err)
		} else {
			metricAnchors.WithLabelValues("file", "ok").Inc()
		}
	}
	if s.opt.AnchorURL != "" {
		if err := s.postAnchor(ctx, a); err != nil {
			metricAnchors.WithLabelValues("http", "error").Inc()
			if firstErr == nil {
				firstErr = fmt.Errorf("anchor url: %w", err)
			}
		} else {
			metricAnchors.WithLabelValues("http", "ok").Inc()
		}
	}
	if firstErr == nil {
		s.lastAnchorSeq, s.lastAnchorHash = tail.Seq, tail.Hash
	}
	return firstErr
}

func appendAnchorFile(path string, a Anchor) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
	}
	b, _ := json.Marshal(a)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (s *Service) postAnchor(ctx context.Context, a Anchor) error {
	b, _ := json.Marshal(a)
	timeout := s.opt.AnchorTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.opt.AnchorURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.opt.AnchorToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.opt.AnchorToken)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// ReadAnchorFile parses an anchor file (JSON lines); unparsable lines are
// skipped.
func ReadAnchorFile(path string) ([]Anchor, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Anchor
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var a Anchor
		if json.Unmarshal(sc.Bytes(), &a) == nil && a.Hash != "" {
			out = append(out, a)
		}
	}
	return out, sc.Err()
}
