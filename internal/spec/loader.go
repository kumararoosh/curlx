package spec

import (
	"fmt"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
)

// Endpoint represents a single API operation from an OpenAPI spec.
type Endpoint struct {
	Method  string
	Path    string
	Summary string
	// OperationID is used as a stable key for the request body cache.
	OperationID string
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
	if v3.Model.Paths != nil {
		for path, item := range v3.Model.Paths.PathItems.FromOldest() {
			ops := map[string]interface{}{
				"GET":    item.Get,
				"POST":   item.Post,
				"PUT":    item.Put,
				"PATCH":  item.Patch,
				"DELETE": item.Delete,
			}
			for method, op := range ops {
				if op == nil {
					continue
				}
				// Use reflection-free approach via type switch.
				ep := extractEndpoint(method, path, op)
				if ep != nil {
					loaded.Endpoints = append(loaded.Endpoints, *ep)
				}
			}
		}
	}

	return loaded, nil
}

func extractEndpoint(method, path string, op interface{}) *Endpoint {
	type hasFields interface {
		GetSummary() string
		GetOperationId() string
	}
	if o, ok := op.(hasFields); ok {
		return &Endpoint{
			Method:      method,
			Path:        path,
			Summary:     o.GetSummary(),
			OperationID: o.GetOperationId(),
		}
	}
	return &Endpoint{Method: method, Path: path}
}
