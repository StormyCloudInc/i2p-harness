package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/catwalk/v3/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/csync"
	"github.com/charmbracelet/crush/internal/oauth"
	"github.com/stretchr/testify/require"
)

func TestRefetchGrokModelsRefreshesExpiredToken(t *testing.T) {
	// Not parallel: swaps the fetchGrokModels package variable.
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
	providers.Set("xai", ProviderConfig{
		ID:         "xai",
		OAuthToken: expired,
		GrokModels: []catwalk.Model{{ID: "grok-stale"}},
	})
	store := &ConfigStore{
		config:         &Config{Providers: providers},
		globalDataPath: configPath,
		exchangeToken: func(ctx context.Context, providerID, refreshToken string) (*oauth.Token, error) {
			require.Equal(t, "xai", providerID)
			require.Equal(t, "expired-rt", refreshToken)
			return fresh, nil
		},
	}

	originalFetch := fetchGrokModels
	var fetchToken *oauth.Token
	fetchGrokModels = func(_ context.Context, token *oauth.Token) ([]catwalk.Model, error) {
		fetchToken = token
		return []catwalk.Model{{ID: "grok-fresh"}}, nil
	}
	t.Cleanup(func() { fetchGrokModels = originalFetch })

	store.refetchGrokModels(t.Context(), ScopeGlobal)

	require.Equal(t, fresh, fetchToken, "the catalog is fetched with the renewed token")

	pc, ok := store.Config().Providers.Get("xai")
	require.True(t, ok)
	require.Equal(t, fresh, pc.OAuthToken, "the renewed token is kept in the config")
	require.Equal(t, "grok-fresh", pc.GrokModels[0].ID, "the existing catalog is replaced")

	disk, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(disk), "grok-fresh", "the fetched catalog is persisted")
	require.Contains(t, string(disk), "fresh-rt", "the renewed token is persisted")
}
