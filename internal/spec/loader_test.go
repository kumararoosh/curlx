package spec

import (
	"sort"
	"testing"
)

// endpointKey is a (method, path) pair used in assertions.
type endpointKey struct{ method, path string }

func keys(eps []Endpoint) []endpointKey {
	out := make([]endpointKey, len(eps))
	for i, e := range eps {
		out[i] = endpointKey{e.Method, e.Path}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].path != out[j].path {
			return out[i].path < out[j].path
		}
		return out[i].method < out[j].method
	})
	return out
}

func mustParse(t *testing.T, yaml string) *LoadedSpec {
	t.Helper()
	s, err := FromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return s
}

// ---------------------------------------------------------------------------
// Method filtering (the core bug: nil *Operation in interface{} != nil)
// ---------------------------------------------------------------------------

func TestMethodFiltering_OnlyDefinedMethodsAppear(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /items:
    get:
      operationId: listItems
      summary: List items
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)

	got := keys(s.Endpoints)
	want := []endpointKey{{"GET", "/items"}}

	if len(got) != len(want) {
		t.Fatalf("expected %d endpoint(s), got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("endpoint[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMethodFiltering_MultipleMethodsSamePath(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /books:
    get:
      operationId: listBooks
      summary: List books
      responses:
        "200":
          description: ok
    post:
      operationId: createBook
      summary: Create book
      responses:
        "201":
          description: created
`
	s := mustParse(t, yaml)

	got := keys(s.Endpoints)
	want := []endpointKey{
		{"GET", "/books"},
		{"POST", "/books"},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d endpoint(s), got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("endpoint[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMethodFiltering_AllFiveMethods(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /resource:
    get:
      responses: {"200": {description: ok}}
    post:
      responses: {"200": {description: ok}}
    put:
      responses: {"200": {description: ok}}
    patch:
      responses: {"200": {description: ok}}
    delete:
      responses: {"200": {description: ok}}
`
	s := mustParse(t, yaml)

	got := keys(s.Endpoints)
	want := []endpointKey{
		{"DELETE", "/resource"},
		{"GET", "/resource"},
		{"PATCH", "/resource"},
		{"POST", "/resource"},
		{"PUT", "/resource"},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d endpoint(s), got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("endpoint[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMethodFiltering_MixedMethodsAcrossPaths(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /auth/login:
    post:
      responses: {"200": {description: ok}}
  /books:
    get:
      responses: {"200": {description: ok}}
    post:
      responses: {"201": {description: created}}
  /books/{id}:
    get:
      responses: {"200": {description: ok}}
    put:
      responses: {"200": {description: ok}}
    delete:
      responses: {"204": {description: deleted}}
`
	s := mustParse(t, yaml)

	got := keys(s.Endpoints)
	want := []endpointKey{
		{"POST", "/auth/login"},
		{"GET", "/books"},
		{"POST", "/books"},
		{"DELETE", "/books/{id}"},
		{"GET", "/books/{id}"},
		{"PUT", "/books/{id}"},
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d endpoint(s), got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("endpoint[%d]: got %v, want %v", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Spec metadata
// ---------------------------------------------------------------------------

func TestParse_Title(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: My API
  version: "2.0"
paths: {}
`
	s := mustParse(t, yaml)
	if s.Title != "My API" {
		t.Errorf("Title: got %q, want %q", s.Title, "My API")
	}
}

func TestParse_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantURL string
	}{
		{
			name: "plain URL",
			yaml: `
openapi: "3.0.3"
info: {title: T, version: "1"}
servers:
  - url: http://localhost:9090
paths: {}
`,
			wantURL: "http://localhost:9090",
		},
		{
			name: "trailing slash stripped",
			yaml: `
openapi: "3.0.3"
info: {title: T, version: "1"}
servers:
  - url: https://api.example.com/v1/
paths: {}
`,
			wantURL: "https://api.example.com/v1",
		},
		{
			name: "no servers entry",
			yaml: `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths: {}
`,
			wantURL: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := mustParse(t, tc.yaml)
			if s.BaseURL != tc.wantURL {
				t.Errorf("BaseURL: got %q, want %q", s.BaseURL, tc.wantURL)
			}
		})
	}
}

func TestParse_NoPaths(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info:
  title: Empty
  version: "1.0"
paths: {}
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 0 {
		t.Errorf("expected no endpoints, got %d", len(s.Endpoints))
	}
}

// ---------------------------------------------------------------------------
// Endpoint metadata
// ---------------------------------------------------------------------------

func TestParse_OperationMetadata(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /ping:
    get:
      operationId: ping
      summary: Health check
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	ep := s.Endpoints[0]
	if ep.OperationID != "ping" {
		t.Errorf("OperationID: got %q, want %q", ep.OperationID, "ping")
	}
	if ep.Summary != "Health check" {
		t.Errorf("Summary: got %q, want %q", ep.Summary, "Health check")
	}
	if ep.Method != "GET" {
		t.Errorf("Method: got %q, want %q", ep.Method, "GET")
	}
	if ep.Path != "/ping" {
		t.Errorf("Path: got %q, want %q", ep.Path, "/ping")
	}
}

func TestParse_MissingOperationMetadata(t *testing.T) {
	// operationId and summary are optional — missing values should be empty strings.
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /ping:
    get:
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	ep := s.Endpoints[0]
	if ep.OperationID != "" {
		t.Errorf("OperationID: got %q, want empty", ep.OperationID)
	}
	if ep.Summary != "" {
		t.Errorf("Summary: got %q, want empty", ep.Summary)
	}
}

// ---------------------------------------------------------------------------
// Parameter extraction
// ---------------------------------------------------------------------------

func TestParse_PathParameter(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /books/{id}:
    get:
      operationId: getBook
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	ep := s.Endpoints[0]
	if len(ep.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(ep.Params))
	}
	p := ep.Params[0]
	if p.Name != "id" {
		t.Errorf("Param.Name: got %q, want %q", p.Name, "id")
	}
	if p.In != "path" {
		t.Errorf("Param.In: got %q, want %q", p.In, "path")
	}
	if !p.Required {
		t.Errorf("Param.Required: got false, want true for path param")
	}
}

func TestParse_QueryParameters(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /search:
    get:
      operationId: search
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
        - name: limit
          in: query
          required: false
          schema:
            type: integer
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	ep := s.Endpoints[0]
	if len(ep.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ep.Params))
	}

	byName := map[string]Param{}
	for _, p := range ep.Params {
		byName[p.Name] = p
	}

	q, ok := byName["q"]
	if !ok {
		t.Fatal("param 'q' not found")
	}
	if q.In != "query" {
		t.Errorf("q.In: got %q, want %q", q.In, "query")
	}
	if !q.Required {
		t.Errorf("q.Required: got false, want true")
	}

	limit, ok := byName["limit"]
	if !ok {
		t.Fatal("param 'limit' not found")
	}
	if limit.Required {
		t.Errorf("limit.Required: got true, want false")
	}
}

func TestParse_PathParamImplicitlyRequired(t *testing.T) {
	// Path params are always required even if the spec omits required: true.
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /items/{id}:
    delete:
      parameters:
        - name: id
          in: path
          schema:
            type: string
      responses:
        "204":
          description: deleted
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	p := s.Endpoints[0].Params[0]
	if !p.Required {
		t.Errorf("path param should be implicitly required even without required:true")
	}
}

func TestParse_NoParams(t *testing.T) {
	const yaml = `
openapi: "3.0.3"
info: {title: T, version: "1"}
paths:
  /status:
    get:
      responses:
        "200":
          description: ok
`
	s := mustParse(t, yaml)
	if len(s.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(s.Endpoints))
	}
	if len(s.Endpoints[0].Params) != 0 {
		t.Errorf("expected no params, got %d", len(s.Endpoints[0].Params))
	}
}
