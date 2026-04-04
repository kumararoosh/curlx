package spec

import (
	"fmt"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// Param represents a single path or query parameter on an endpoint.
type Param struct {
	Name     string
	In       string // "path" or "query"
	Required bool
}

// Endpoint represents a single API operation from an OpenAPI spec.
type Endpoint struct {
	Method  string
	Path    string
	Summary string
	// OperationID is used as a stable key for the request body cache.
	OperationID string
	// Params holds path and query parameters defined in the spec.
	Params []Param
}

// LoadedSpec is the parsed result of an OpenAPI document.
type LoadedSpec struct {
	Title     string
	BaseURL   string
	Endpoints []Endpoint
	Source    string // original input string used to load this spec
}

// FromFile loads and parses an OpenAPI spec from a local file path.
func FromFile(path string) (*LoadedSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file: %w", err)
	}
	return parse(data)
}

// FromBytes parses an OpenAPI spec from raw bytes (e.g. fetched from git).
func FromBytes(data []byte) (*LoadedSpec, error) {
	return parse(data)
}

func parse(data []byte) (*LoadedSpec, error) {
	doc, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parsing OpenAPI document: %w", err)
	}

	v3, errs := doc.BuildV3Model()
	if len(errs) > 0 {
		// Non-fatal warnings are common; only fail on hard errors.
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return nil, fmt.Errorf("building OpenAPI model: %s", strings.Join(msgs, "; "))
	}

	loaded := &LoadedSpec{
		Title: v3.Model.Info.Title,
	}

	// Extract base URL from the first server entry.
	if len(v3.Model.Servers) > 0 {
		loaded.BaseURL = strings.TrimRight(v3.Model.Servers[0].URL, "/")
	}

	// Walk all paths and methods.
	// Each operation field is a *v3high.Operation; a nil pointer stored in an
	// interface{} is not equal to nil, so we keep typed pairs to get correct
	// nil checks.
	type methodOp struct {
		method string
		op     *v3high.Operation
	}
	if v3.Model.Paths != nil {
		for path, item := range v3.Model.Paths.PathItems.FromOldest() {
			candidates := []methodOp{
				{"GET", item.Get},
				{"POST", item.Post},
				{"PUT", item.Put},
				{"PATCH", item.Patch},
				{"DELETE", item.Delete},
			}
			for _, c := range candidates {
				if c.op == nil {
					continue
				}
				loaded.Endpoints = append(loaded.Endpoints, extractEndpoint(c.method, path, c.op))
			}
		}
	}

	return loaded, nil
}

func extractEndpoint(method, path string, op *v3high.Operation) Endpoint {
	ep := Endpoint{
		Method:      method,
		Path:        path,
		Summary:     op.Summary,
		OperationID: op.OperationId,
	}
	for _, p := range op.Parameters {
		if p == nil {
			continue
		}
		required := p.In == "path" // path params are implicitly required
		if p.Required != nil {
			required = *p.Required
		}
		ep.Params = append(ep.Params, Param{
			Name:     p.Name,
			In:       p.In,
			Required: required,
		})
	}
	return ep
}
