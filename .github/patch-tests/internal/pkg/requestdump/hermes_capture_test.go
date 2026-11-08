//go:build unit

package requestdump

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func TestHermesPatchCaptureOptInAndRequestCorrelation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dumps")
	t.Setenv("SUB2API_REQUEST_DUMP_DIR", dir)
	t.Setenv("SUB2API_REQUEST_DUMP_ENABLED", "false")
	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/v1internal:streamGenerateContent?alt=sse", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer audit-header-value")
	req.Header.Set("Cookie", "audit-cookie-value")
	req.Header.Set("X-Api-Key", "audit-key-value")
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "../audit/id")
	body := []byte(`{"request":{"contents":[{"parts":[{"text":"diagnostic text"}]}],"nested":{"access_token":"fixture-access","refresh_token":"fixture-refresh","password":"fixture-password"}}}`)
	if err := Capture(ctx, "streamGenerateContent", req, body); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("capture created a directory while disabled")
	}
	t.Setenv("SUB2API_REQUEST_DUMP_ENABLED", "true")
	if err := Capture(context.Background(), "streamGenerateContent", req, body); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("capture wrote data without a request id")
	}
	for i := 0; i < 2; i++ {
		if err := Capture(ctx, "streamGenerateContent", req, body); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("continuous capture should retain both attempts: %d", len(files))
	}
	for _, file := range files {
		name := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"audit-header-value", "audit-cookie-value", "audit-key-value", "fixture-access", "fixture-refresh", "fixture-password"} {
			if strings.Contains(string(data), secret) {
				t.Errorf("credential field was not redacted: %s", secret)
			}
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["request_id"] != "../audit/id" || payload["action"] != "streamGenerateContent" {
			t.Error("correlation metadata was lost")
		}
		if !strings.Contains(string(data), "diagnostic text") {
			t.Error("request contents were not captured")
		}
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("unexpected dump file mode: %o", info.Mode().Perm())
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("unexpected directory mode: %o", info.Mode().Perm())
	}
}
