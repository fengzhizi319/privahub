// Package apidocs serves the generated OpenAPI 3 document of the
// /api/v1alpha1 routes (the Go counterpart of Java springdoc).
//
// openapi.json is generated from the live gin route table plus the handler
// request/response types; regenerate it after changing routes or DTOs:
//
//	CGO_ENABLED=1 go run ./cmd/openapi-gen -out internal/controller/http/apidocs/openapi.json
package apidocs

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.json
var document []byte

// Paths returns the set of "METHOD path" keys described by the document.
func Paths() (map[string]bool, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(document, &doc); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for p, ops := range doc.Paths {
		for m := range ops {
			out[httpMethod(m)+" "+p] = true
		}
	}
	return out, nil
}

func httpMethod(m string) string {
	switch m {
	case "get":
		return http.MethodGet
	case "post":
		return http.MethodPost
	case "put":
		return http.MethodPut
	case "delete":
		return http.MethodDelete
	}
	return m
}

// Handler serves the document as application/json.
func Handler(c *gin.Context) {
	c.Data(http.StatusOK, "application/json; charset=utf-8", document)
}
