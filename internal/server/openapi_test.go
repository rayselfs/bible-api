package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}

	var spec struct {
		OpenAPI string                          `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI version = %q, want 3.1.0", spec.OpenAPI)
	}

	want := map[string]string{
		"/health":                            "get",
		"/api/bible/v1/versions":             "get",
		"/api/bible/v1/content/{version_id}": "get",
		"/api/bible/v1/vectors/{version_id}": "get",
		"/api/bible/v1/verse/{id}":           "post",
		"/api/bible/v2/versions":             "get",
	}
	if len(spec.Paths) != len(want) {
		t.Fatalf("documented paths = %d, want %d", len(spec.Paths), len(want))
	}
	for path, method := range want {
		operations := spec.Paths[path]
		if _, ok := operations[method]; !ok {
			t.Errorf("missing %s %s", method, path)
			continue
		}
		if len(operations) != 1 {
			t.Errorf("documented methods for %s = %d, want only %s", path, len(operations), method)
		}
	}
}
