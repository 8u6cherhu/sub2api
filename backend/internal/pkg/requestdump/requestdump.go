// Package requestdump provides an opt-in, single-request diagnostic capture.
package requestdump

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

const (
	requestIDEnv = "SUB2API_REQUEST_DUMP_REQUEST_ID"
	dirEnv       = "SUB2API_REQUEST_DUMP_DIR"
	defaultDir   = "/app/data/request-dumps"
	maxBodyBytes = 4 << 20
)

var captured sync.Map

// Capture writes one redacted outbound request when its request ID exactly
// matches SUB2API_REQUEST_DUMP_REQUEST_ID. An empty setting disables capture.
func Capture(ctx context.Context, action string, req *http.Request, body []byte) error {
	target := strings.TrimSpace(os.Getenv(requestIDEnv))
	requestID, _ := ctx.Value(ctxkey.RequestID).(string)
	requestID = strings.TrimSpace(requestID)
	if target == "" || requestID == "" || requestID != target || req == nil {
		return nil
	}
	key := requestID + "\x00" + action
	if _, loaded := captured.LoadOrStore(key, struct{}{}); loaded {
		return nil
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes]
	}

	decoded := any(string(body))
	var value any
	if json.Unmarshal(body, &value) == nil {
		decoded = redact(value)
	}
	envelope := map[string]any{
		"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
		"request_id":  requestID,
		"action":      action,
		"method":      req.Method,
		"url":         req.URL.String(),
		"headers":     redactHeaders(req.Header),
		"body":        decoded,
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return err
	}
	dir := strings.TrimSpace(os.Getenv(dirEnv))
	if dir == "" {
		dir = defaultDir
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	name := fmt.Sprintf("%s-%s.json", safe(requestID), safe(action))
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(encoded)
	return err
}

func redactHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		if sensitive(key) {
			result[key] = []string{"[REDACTED]"}
		} else {
			result[key] = append([]string(nil), values...)
		}
	}
	return result
}

func redact(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if sensitive(key) {
				typed[key] = "[REDACTED]"
			} else {
				typed[key] = redact(item)
			}
		}
	case []any:
		for index := range typed {
			typed[index] = redact(typed[index])
		}
	}
	return value
}

func sensitive(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), "_", ""))
	for _, marker := range []string{"authorization", "accesstoken", "refreshtoken", "apikey", "clientsecret", "password", "cookie", "secret", "credential"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func safe(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('_')
		}
	}
	return builder.String()
}
