// Copyright 2026 The Privahub Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command healthcheck is a tiny static probe used by the container
// HEALTHCHECK. The runtime image is distroless (no shell, curl or wget), so
// the probe ships as its own binary. It only talks to loopback.
//
// Environment:
//
//	PRIVAHUB_HEALTHCHECK_URL  default http://127.0.0.1:8080/api/v1alpha1/healthz
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const defaultURL = "http://127.0.0.1:8080/api/v1alpha1/healthz"

func main() {
	raw := os.Getenv("PRIVAHUB_HEALTHCHECK_URL")
	if raw == "" {
		raw = defaultURL
	}
	if err := probe(raw); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
}

func probe(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("refusing non-loopback target %q", u.Host)
	}
	tr := &http.Transport{
		// The probe only ever connects to loopback (enforced above) and reads
		// a liveness status; the server certificate is issued for the public
		// hostname, so verification against 127.0.0.1 cannot succeed.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec // loopback-only liveness probe, see comment above
		Proxy:           nil,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //nolint:gosec // G704: target restricted to loopback above
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Transport: tr}).Do(req) //nolint:gosec // G704: target restricted to loopback above
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
