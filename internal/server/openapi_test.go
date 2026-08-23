package server

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type openAPISpec struct {
	OpenAPI    string                          `yaml:"openapi"`
	Service    string                          `yaml:"x-hhc-service"`
	Owner      string                          `yaml:"x-hhc-owner"`
	Repository string                          `yaml:"x-hhc-repository"`
	Paths      map[string]map[string]operation `yaml:"paths"`
	Components struct {
		Parameters map[string]parameter `yaml:"parameters"`
	} `yaml:"components"`
}

type operation struct {
	Tags        []string            `yaml:"tags"`
	Visibility  string              `yaml:"x-hhc-visibility"`
	Callers     []string            `yaml:"x-hhc-callers"`
	Description string              `yaml:"description"`
	Responses   map[string]response `yaml:"responses"`
	SSEFrame    sseFrame            `yaml:"x-hhc-sse-frame"`
	SSEEvents   []sseEvent          `yaml:"x-hhc-sse-events"`
}

type parameter struct {
	Description string `yaml:"description"`
}

type response struct {
	Content map[string]yaml.Node `yaml:"content"`
}

type sseEvent struct {
	Name   string   `yaml:"name"`
	Fields []string `yaml:"fields"`
}

type sseFrame struct {
	Fields []string `yaml:"fields"`
	ID     string   `yaml:"id"`
}

func TestOpenAPICoversRegisteredRoutes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}

	var spec openAPISpec
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

	if spec.Service != "bible-api" || spec.Owner != "HHC Platform" || spec.Repository != "rayselfs/bible-api" {
		t.Errorf("catalog metadata = service %q, owner %q, repository %q", spec.Service, spec.Owner, spec.Repository)
	}

	wantOperations := map[string]struct {
		tag        string
		visibility string
		callers    []string
	}{
		"/health get":                            {"Operations", "operations", []string{}},
		"/api/bible/v1/versions get":             {"Public", "public", []string{"hhc-client"}},
		"/api/bible/v1/content/{version_id} get": {"Public", "public", []string{"hhc-client", "hhc-client-v2"}},
		"/api/bible/v1/vectors/{version_id} get": {"Public", "public", []string{}},
		"/api/bible/v1/verse/{id} post":          {"Private", "private", []string{}},
		"/api/bible/v2/versions get":             {"Public", "public", []string{"hhc-client-v2"}},
	}
	for route, want := range wantOperations {
		parts := strings.Split(route, " ")
		op := spec.Paths[parts[0]][parts[1]]
		if !reflect.DeepEqual(op.Tags, []string{want.tag}) || op.Visibility != want.visibility || !reflect.DeepEqual(op.Callers, want.callers) {
			t.Errorf("%s metadata = tags %v, visibility %q, callers %v", route, op.Tags, op.Visibility, op.Callers)
		}
	}

	for name, want := range map[string]string{
		"UserID":      "The public API gateway clears this header. Direct and internal callers can provide an unverified value; middleware copies it without authentication or authorization.",
		"Roles":       "The public API gateway clears this header. Direct and internal callers can provide an unverified value; middleware copies it without authentication or authorization.",
		"Permissions": "The public API gateway clears this header. Direct and internal callers can provide an unverified value; middleware copies it without authentication or authorization.",
	} {
		if got := spec.Components.Parameters[name].Description; !strings.Contains(got, want) {
			t.Errorf("%s header description = %q, want gateway-cleared and direct/internal unverified semantics", name, got)
		}
	}

	content := spec.Paths["/api/bible/v1/content/{version_id}"]["get"]
	if _, ok := content.Responses["200"].Content["text/event-stream"]; !ok {
		t.Error("content 200 must be text/event-stream")
	}
	if _, ok := content.Responses["500"]; ok {
		t.Error("content must not document unreachable 500 response")
	}
	if !reflect.DeepEqual(content.SSEFrame.Fields, []string{"id", "data"}) || !strings.Contains(content.SSEFrame.ID, "id: <incrementing integer>") {
		t.Errorf("content SSE frame = fields %v, id %q", content.SSEFrame.Fields, content.SSEFrame.ID)
	}
	wantSSEEvents := []sseEvent{
		{Name: "start", Fields: []string{"type", "message"}},
		{Name: "header", Fields: []string{"version_id", "version_code", "version_name", "updated_at", "books"}},
		{Name: "book", Fields: []string{"id", "number", "name", "abbreviation", "chapters"}},
		{Name: "complete", Fields: []string{"type", "total_books", "message"}},
		{Name: "error", Fields: []string{"type", "message"}},
		{Name: "timeout", Fields: []string{"type", "message"}},
	}
	if !reflect.DeepEqual(content.SSEEvents, wantSSEEvents) || !strings.Contains(content.Description, "already-started 200") {
		t.Errorf("content streaming contract = events %v, description %q", content.SSEEvents, content.Description)
	}

	vectors := spec.Paths["/api/bible/v1/vectors/{version_id}"]["get"]
	if _, ok := vectors.Responses["200"].Content["application/octet-stream"]; !ok {
		t.Error("vectors 200 must be application/octet-stream")
	}
	if _, ok := vectors.Responses["500"]; ok {
		t.Error("vectors must not document unreachable 500 response")
	}
	if !strings.Contains(vectors.Description, "terminates the already-started 200 stream without a terminal record") {
		t.Errorf("vectors streaming termination = %q", vectors.Description)
	}
}
