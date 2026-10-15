// Copyright 2026 Privahub Authors. Licensed under the Apache License, Version 2.0.
package http

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// The baseline comes from the Java source, independently of the Go router and
// generated OpenAPI document. A route must stay on its intended listener; moving
// node RPCs onto the public router is a security regression, not API coverage.
func TestMigration_LegacyRouteCoverage(t *testing.T) {
	raw, err := os.ReadFile("testdata/legacy_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	var legacy []struct{ Controller, Method, Path, Listener string }
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if len(legacy) == 0 {
		t.Fatal("empty legacy contract")
	}
	app := newTestApp(t)
	routers := map[string]*gin.Engine{"main": NewRouter(zap.NewNop(), app), "inner": NewInnerRouter(zap.NewNop(), app)}
	registered := map[string]map[string]bool{}
	for listener, router := range routers {
		registered[listener] = map[string]bool{}
		for _, route := range router.Routes() {
			registered[listener][route.Method+" "+route.Path] = true
		}
	}
	for _, route := range legacy {
		key := route.Method + " " + route.Path
		if !registered[route.Listener][key] {
			t.Errorf("%s: missing %s on %s listener", route.Controller, key, route.Listener)
		}
		if route.Listener == "inner" {
			// A shared registration is allowed only when BlockInnerPaths hides
			// it on the main listener before authentication/handler execution.
			code, _ := getStatus(t, routers["main"], route.Method, route.Path, "127.0.0.1:1", nil, `{}`)
			if code != http.StatusNotFound {
				t.Errorf("internal RPC reachable publicly: %s (HTTP %d)", key, code)
			}
		}
	}
}
