package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/catwalk/v3/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/oauth"
	"github.com/stretchr/testify/require"
)

func TestGetModelIncludesChatGPTModels(t *testing.T) {
	t.Parallel()

	providers := csync.NewMap[string, ProviderConfig]()
	providers.Set("openai", ProviderConfig{
		Models:        []catwalk.Model{{ID: "gpt-5.1"}},
		ChatGPTModels: []catwalk.Model{{ID: "gpt-5.1-codex", Name: "GPT-5.1 Codex"}},
	})
	cfg := &Config{Providers: providers}

	model := cfg.GetModel("openai", "gpt-5.1-codex")
	require.NotNil(t, model)
	require.Equal(t, "GPT-5.1 Codex", model.Name)

	require.NotNil(t, cfg.GetModel("openai", "gpt-5.1"))
	require.Nil(t, cfg.GetModel("openai", "missing"))
}

func TestTokenFields(t *testing.T) {
	t.Parallel()

	t.Run("openai keeps the api key", func(t *testing.T) {
		t.Parallel()

		fields := tokenFields("openai", &oauth.Token{AccessToken: "at"})
		require.Contains(t, fields, "providers.openai.oauth")
		require.NotContains(t, fields, "providers.openai.api_key")
	})

	t.Run("other providers mirror the access token", func(t *testing.T) {
		t.Parallel()

		fields := tokenFields("hyper", &oauth.Token{AccessToken: "at"})
		require.Contains(t, fields, "providers.hyper.oauth")
		require.Equal(t, "at", fields["providers.hyper.api_key"])
	})
}

func TestTokenAccountIDRoundTrip(t *testing.T) {
	t.Parallel()

	// The account ID is persisted with the token in crush.json; verify
	// it survives the JSON round trip loadTokenFromDisk performs.
	token := &oauth.Token{
		AccessToken:  "at",
		RefreshToken: "rt",
		IDToken:      "idt",
		AccountID:    "acct-42",
		ExpiresIn:    3600,
		ExpiresAt:    4102444800,
	}

	data, err := json.Marshal(token)
	require.NoError(t, err)

	var restored oauth.Token
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, "acct-42", restored.AccountID)
	require.Equal(t, "idt", restored.IDToken)
	require.Equal(t, "rt", restored.RefreshToken)
}

// testResolver mimics the shell resolver for templates: variables
// resolve to empty when unset, literals pass through.
type testResolver struct{}

func (testResolver) ResolveValue(v string) (string, error) {
	if strings.HasPrefix(v, "$") {
		return "", nil
	}
	return v, nil
}

func TestHasAPIKey(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		require.False(t, (&ProviderConfig{}).HasAPIKey(testResolver{}))
	})

	t.Run("literal key", func(t *testing.T) {
		t.Parallel()

		require.True(t, (&ProviderConfig{APIKey: "sk-key"}).HasAPIKey(testResolver{}))
	})

	t.Run("unresolved template", func(t *testing.T) {
		t.Parallel()

		require.False(t, (&ProviderConfig{APIKey: "$OPENAI_API_KEY"}).HasAPIKey(testResolver{}))
	})
}

func TestRefetchOpenAIModelsRefreshesExpiredToken(t *testing.T) {
	// Not parallel: swaps the fetchOpenAIModels package variable.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "crush.json")

	expired := &oauth.Token{
		AccessToken:  "expired-at",
		RefreshToken: "expired-rt",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(-time.Hour).Unix(),
	}
	fresh := &oauth.Token{
		AccessToken:  "fresh-at",
		RefreshToken: "fresh-rt",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}

	providers := csync.NewMap[string, ProviderConfig]()
	providers.Set("openai", ProviderConfig{
		ID:            "openai",
		OAuthToken:    expired,
		ChatGPTModels: []catwalk.Model{{ID: "gpt-stale"}},
	})
	store := &ConfigStore{
		config:         &Config{Providers: providers},
		globalDataPath: configPath,
		exchangeToken: func(ctx context.Context, providerID, refreshToken string) (*oauth.Token, error) {
			require.Equal(t, "openai", providerID)
			require.Equal(t, "expired-rt", refreshToken)
			return fresh, nil
		},
	}

	originalFetch := fetchOpenAIModels
	var fetchToken *oauth.Token
	fetchOpenAIModels = func(_ context.Context, token *oauth.Token) ([]catwalk.Model, error) {
		fetchToken = token
		return []catwalk.Model{{ID: "gpt-fresh"}}, nil
	}
	t.Cleanup(func() { fetchOpenAIModels = originalFetch })

	store.refetchOpenAIModels(t.Context(), ScopeGlobal)

	require.Equal(t, fresh, fetchToken, "the catalog is fetched with the renewed token")

	pc, ok := store.Config().Providers.Get("openai")
	require.True(t, ok)
	require.Equal(t, fresh, pc.OAuthToken, "the renewed token is kept in the config")
	require.Equal(t, "gpt-fresh", pc.ChatGPTModels[0].ID, "the existing catalog is replaced")

	disk, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(disk), "gpt-fresh", "the fetched catalog is persisted")
	require.Contains(t, string(disk), "fresh-rt", "the renewed token is persisted")
}
