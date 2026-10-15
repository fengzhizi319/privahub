package graphjob

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The serving AppImage name is a contract with the dev deployment: clwork
// scripts/dev-start.sh registers kuscia/scripts/templates/app_image.serving.yaml
// as SERVING_APP_IMAGE_NAME (default "sf-serving-image"). model/serving/create
// fails with "appimage not found" when they drift apart.
func TestServingImageMatchesDevAppImage(t *testing.T) {
	const want = "sf-serving-image"
	if got := DefaultSettings().ServingImage; got != want {
		t.Fatalf("default serving_image = %q, want %q", got, want)
	}
	graphYAML := filepath.Join("..", "..", "config", "graph.yaml")
	if _, err := os.Stat(graphYAML); err != nil {
		t.Fatalf("config/graph.yaml: %v", err)
	}
	if got := LoadSettings(graphYAML).ServingImage; got != want {
		t.Fatalf("config/graph.yaml serving_image = %q, want %q", got, want)
	}

	// Cross-check the clwork workspace when present (privahub cloned standalone skips).
	root := filepath.Join("..", "..", "..")
	devStart, err := os.ReadFile(filepath.Join(root, "scripts", "dev-start.sh"))
	if err != nil {
		t.Skip("clwork workspace not present")
	}
	if !regexp.MustCompile(`SERVING_APP_IMAGE_NAME="\$\{SERVING_APP_IMAGE_NAME:-` + want + `\}"`).Match(devStart) {
		t.Errorf("scripts/dev-start.sh does not default SERVING_APP_IMAGE_NAME to %q", want)
	}
	if _, err := os.Stat(filepath.Join(root, "kuscia", "scripts", "templates", "app_image.serving.yaml")); err != nil {
		t.Errorf("kuscia serving AppImage template missing: %v", err)
	}
}
