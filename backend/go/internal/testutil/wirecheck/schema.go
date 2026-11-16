// Package wirecheck validates real HTTP and SSE bytes against checked-in schemas.
// It is a test-only consumer; production code never imports the validator.
package wirecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type contract struct {
	pattern   string
	Request   string            `json:"request"`
	Responses map[string]string `json:"responses"`
}
type manifest struct {
	HTTP map[string]contract          `json:"http"`
	SSE  map[string]map[string]string `json:"sse"`
}

type validator struct {
	schemas   map[string]*jsonschema.Schema
	contracts manifest
	err       error
}

var cached = sync.OnceValue(loadSchemas)

func loadSchemas() validator {
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "../../../../../docs/api/schema")
	v := validator{schemas: map[string]*jsonschema.Schema{}}
	raw, err := os.ReadFile(filepath.Join(dir, "contracts.json"))
	if err != nil {
		v.err = err
		return v
	}
	if err = json.Unmarshal(raw, &v.contracts); err != nil {
		v.err = err
		return v
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		v.err = err
		return v
	}
	c := jsonschema.NewCompiler()
	for _, file := range files {
		if filepath.Base(file) == "contracts.json" {
			continue
		}
		raw, err = os.ReadFile(file)
		if err != nil {
			v.err = err
			return v
		}
		doc, decodeErr := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if decodeErr != nil {
			v.err = decodeErr
			return v
		}
		if err = c.AddResource(file, doc); err != nil {
			v.err = err
			return v
		}
	}
	for _, file := range files {
		if filepath.Base(file) == "contracts.json" {
			continue
		}
		schema, compileErr := c.Compile(file)
		if compileErr != nil {
			v.err = compileErr
			return v
		}
		v.schemas[strings.TrimSuffix(filepath.Base(file), ".json")] = schema
	}
	return v
}

// Validate accepts JSON bytes, decoded JSON, or a typed fixture. Unknown schema
// names and malformed JSON fail explicitly rather than disabling a contract.
func Validate(name string, body any) error {
	v := cached()
	if v.err != nil {
		return v.err
	}
	schema, exists := v.schemas[name]
	if !exists {
		return fmt.Errorf("unknown wire schema %q", name)
	}
	raw, ok := body.([]byte)
	if !ok {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

// AssertSchema checks structure while the caller keeps its semantic assertions.
func AssertSchema(t testing.TB, name string, body any) {
	t.Helper()
	if err := Validate(name, body); err != nil {
		t.Errorf("wire schema %s: %v", name, err)
	}
}
