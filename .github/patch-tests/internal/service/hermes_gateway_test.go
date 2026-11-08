//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHermesPatchClientSearchAcrossMappedModelFamilies(t *testing.T) {
	for _, model := range []string{"gemini-3.1-pro-high", "gemini-3.5-flash", "claude-sonnet-4-6"} {
		t.Run(model, func(t *testing.T) {
			body := []byte(`{"messages":[{"role":"user","content":"find documentation"}],"tools":[{"name":"web_search","input_schema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}},{"name":"terminal","input_schema":{"type":"object"}}]}`)
			var claude antigravity.ClaudeRequest
			require.NoError(t, json.Unmarshal(body, &claude))
			svc := &AntigravityGatewayService{}
			got, err := svc.buildAntigravityCompatGeminiBody(context.Background(), body, &claude, "audit-project", model)
			require.NoError(t, err)
			functions := map[string]bool{}
			for _, tool := range gjson.GetBytes(got, "request.tools").Array() {
				for _, decl := range tool.Get("functionDeclarations").Array() {
					functions[decl.Get("name").String()] = true
				}
				require.False(t, tool.Get("googleSearch").Exists(), "client search is not a server builtin")
			}
			require.True(t, functions["terminal"])
			require.True(t, functions["web_search"], "the client function must remain callable")
			require.Equal(t, "agent", gjson.GetBytes(got, "requestType").String())
			require.Equal(t, model, gjson.GetBytes(got, "model").String())
		})
	}
}

func TestHermesPatchNativeGeminiNormalizationReachesTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, role, tools string
		mixed             bool
	}{
		{name: "omitted_role", tools: `[{"googleSearch":{}}]`},
		{name: "snake_case_search", role: `"role":"user",`, tools: `[{"google_search":{}}]`},
		{name: "snake_case_search_with_function", role: `"role":"user",`, tools: `[{"google_search":{}},{"functionDeclarations":[{"name":"terminal","parameters":{"type":"object"}}]}]`, mixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"contents":[{` + tc.role + `"parts":[{"text":"hello"}]},{"role":"model","parts":[{"text":"previous answer"}]},{"role":"user","parts":[{"text":"continue"}]}],"tools":` + tc.tools + `}`)
			var forwarded []byte
			upstream := &hermesPatchHTTPUpstream{
				onCall: func(req *http.Request) {
					var err error
					forwarded, err = io.ReadAll(req.Body)
					require.NoError(t, err)
				},
			}
			svc := NewAntigravityGatewayService(nil, nil, nil, &AntigravityTokenProvider{}, nil, upstream,
				NewSettingService(nil, &config.Config{}), nil)
			account := &Account{
				ID: 1, Name: "fixture", Platform: PlatformAntigravity, Type: AccountTypeOAuth,
				Status: StatusActive, Concurrency: 1,
				Credentials: map[string]any{
					"access_token":  "fixture-access",
					"project_id":    "fixture-project",
					"model_mapping": map[string]any{"gemini-3.1-pro-high": "gemini-3.1-pro-high"},
				},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.1-pro-high:generateContent", bytes.NewReader(body))
			_, err := svc.ForwardGemini(context.Background(), c, account, "gemini-3.1-pro-high", "generateContent", false, body, false)
			require.NoError(t, err)
			require.Equal(t, "user", gjson.GetBytes(forwarded, "request.contents.0.role").String(), "omitted role must be normalized before transport")
			require.Equal(t, "model", gjson.GetBytes(forwarded, "request.contents.1.role").String())
			serverSearch := false
			hasFunction := false
			for _, tool := range gjson.GetBytes(forwarded, "request.tools").Array() {
				require.False(t, tool.Get("google_search").Exists(), "snake-case alias must be normalized")
				serverSearch = serverSearch || tool.Get("googleSearch").Exists()
				hasFunction = hasFunction || tool.Get("functionDeclarations.0.name").String() == "terminal"
			}
			require.Equal(t, !tc.mixed, serverSearch, "normalization must also feed mixed-tool reconciliation")
			require.Equal(t, tc.mixed, hasFunction, "reconciliation must preserve client functions")
		})
	}
}

// Keep transport fixtures local to the fork instead of depending on upstream's
// test helper names or performing any external API requests.
type hermesPatchHTTPUpstream struct {
	onCall func(*http.Request)
}

func (s *hermesPatchHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.onCall(req)
	body := `data: {"response":{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}}` + "\n\n"
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (s *hermesPatchHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}
