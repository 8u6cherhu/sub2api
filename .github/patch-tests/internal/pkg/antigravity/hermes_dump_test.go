//go:build unit

package antigravity

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestHermesPatchDumpIsWiredIntoOutboundRequest(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUB2API_REQUEST_DUMP_ENABLED", "true")
	t.Setenv("SUB2API_REQUEST_DUMP_DIR", dir)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "audit-request")
	body := []byte(`{"request":{"contents":[{"role":"user","parts":[{"text":"fixture"}]}]}}`)
	req, err := NewAPIRequestWithURL(ctx, "https://example.invalid", "streamGenerateContent", "audit-bearer-value", body)
	if err != nil {
		t.Fatal(err)
	}
	forwarded, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(forwarded) != string(body) || req.Header.Get("Authorization") != "Bearer audit-bearer-value" {
		t.Error("capturing diagnostics altered the actual upstream request")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("outbound request did not produce one capture: %v %d", err, len(files))
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "audit-bearer-value") || !strings.Contains(string(data), "[REDACTED]") {
		t.Error("the stored request did not redact its bearer header")
	}
}

func TestHermesPatchDumpFailureDoesNotBlockRequest(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUB2API_REQUEST_DUMP_ENABLED", "true")
	t.Setenv("SUB2API_REQUEST_DUMP_DIR", filepath.Join(blocker, "dumps"))
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "audit-request")
	req, err := NewAPIRequestWithURL(ctx, "https://example.invalid", "streamGenerateContent", "audit-value", []byte(`{}`))
	if err != nil || req == nil {
		t.Fatalf("an optional diagnostic failure blocked the upstream request: %v", err)
	}
}
