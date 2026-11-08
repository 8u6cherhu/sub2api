//go:build unit

package antigravity

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHermesPatchClientSearchRetainsFunctionAndAgentRoute(t *testing.T) {
	input := &ClaudeRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"find the documentation"`)}},
		Tools: []ClaudeTool{
			{Name: "web_search", InputSchema: map[string]any{
				"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}},
				"required": []any{"query"},
			}},
			{Name: "terminal", InputSchema: map[string]any{"type": "object"}},
		},
	}
	body, err := TransformClaudeToGeminiWithOptions(input, "audit-project", input.Model, DefaultTransformOptions())
	if err != nil {
		t.Fatal(err)
	}
	var got V1InternalRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	functions := map[string]GeminiFunctionDecl{}
	for _, tool := range got.Request.Tools {
		for _, decl := range tool.FunctionDeclarations {
			functions[decl.Name] = decl
		}
		if tool.GoogleSearch != nil {
			t.Error("a client function was promoted to a server-side search")
		}
	}
	if _, ok := functions["web_search"]; !ok {
		t.Error("the client's executable web_search function was removed")
	} else if !reflect.DeepEqual(functions["web_search"].Parameters, input.Tools[0].InputSchema) {
		t.Error("the client's web_search parameter schema was changed")
	}
	if _, ok := functions["terminal"]; !ok {
		t.Error("the client's terminal function was removed")
	}
	if got.RequestType != "agent" {
		t.Errorf("ordinary client functions must keep agent routing, got %q", got.RequestType)
	}
	if got.Model != input.Model {
		t.Errorf("ordinary client functions changed the selected model to %q", got.Model)
	}
}

func TestHermesPatchBuiltinSearchRemainsServerExecuted(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "builtin_only"
		tools := []ClaudeTool{{Type: "web_search_20250305", Name: "web_search"}}
		if mixed {
			name = "builtin_with_client_function"
			tools = append(tools, ClaudeTool{Name: "terminal", InputSchema: map[string]any{"type": "object"}})
		}
		t.Run(name, func(t *testing.T) {
			body, err := TransformClaudeToGeminiWithOptions(&ClaudeRequest{
				Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"find the documentation"`)}},
				Tools:    tools,
			}, "fixture-project", "claude-sonnet-4-6", DefaultTransformOptions())
			if err != nil {
				t.Fatal(err)
			}
			var got V1InternalRequest
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatal(err)
			}
			hasServerSearch := false
			for _, tool := range got.Request.Tools {
				hasServerSearch = hasServerSearch || tool.GoogleSearch != nil
				for _, decl := range tool.FunctionDeclarations {
					if decl.Name == "web_search" {
						t.Error("server-executed search became a client function with no client implementation")
					}
				}
			}
			if !mixed && !hasServerSearch {
				t.Error("a search-only request lost server-executed search")
			}
			if mixed && hasServerSearch {
				t.Error("v1internal received a known-invalid built-in/function mix")
			}
		})
	}
}

func TestHermesPatchMacOSUserAgent(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"2.9.0", "antigravity/2.9.0 darwin/arm64"},
		{"2.9.1", "antigravity/hub/2.9.1 darwin/arm64"},
		{"2.10.0", "antigravity/hub/2.10.0 darwin/arm64"},
		{"3.0.0", "antigravity/hub/3.0.0 darwin/arm64"},
	} {
		if got := BuildUserAgent(tc.version); got != tc.want {
			t.Errorf("BuildUserAgent(%q) = %q, want %q", tc.version, got, tc.want)
		}
	}
}
