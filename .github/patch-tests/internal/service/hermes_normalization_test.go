//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHermesPatchNativeNormalizationPreservesExistingValues(t *testing.T) {
	body := []byte(`{"contents":[{"parts":[]},{"role":null,"parts":[]},{"role":"","parts":[]},{"role":"  ","parts":[]},{"role":"model","parts":[]}],"tools":[{"google_search":{"ignored":true},"googleSearch":{"kept":true}},{"google_search":{}}],"generationConfig":{"temperature":0.3}}`)
	got, err := normalizeGeminiRequestForAntigravity(body)
	require.NoError(t, err)
	for _, index := range []string{"0", "1", "2", "3"} {
		require.Equal(t, "user", gjson.GetBytes(got, "contents."+index+".role").String())
	}
	require.Equal(t, "model", gjson.GetBytes(got, "contents.4.role").String())
	require.True(t, gjson.GetBytes(got, "tools.0.googleSearch.kept").Bool())
	require.False(t, gjson.GetBytes(got, "tools.0.google_search").Exists())
	require.True(t, gjson.GetBytes(got, "tools.1.googleSearch").Exists())
	require.Equal(t, 0.3, gjson.GetBytes(got, "generationConfig.temperature").Float())
	again, err := normalizeGeminiRequestForAntigravity(got)
	require.NoError(t, err)
	require.Equal(t, string(got), string(again), "normalization should be idempotent")
}
