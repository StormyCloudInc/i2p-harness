package agent

import (
	"testing"

	"charm.land/catwalk/v3/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

func TestServingProvider(t *testing.T) {
	t.Parallel()

	providerCfg := func(id string, providerType catwalk.Type) config.ProviderConfig {
		return config.ProviderConfig{ID: id, Type: providerType}
	}
	model := func(modelType catwalk.Type) catwalk.Model {
		return catwalk.Model{ID: "some-model", Type: modelType}
	}

	t.Run("completions ignore former built-in provider IDs", func(t *testing.T) {
		t.Parallel()
		for _, providerID := range []string{"bedrock", "azure", "gemini", "openrouter", "vercel", "hyper", "copilot"} {
			require.Equal(t, "openai-compat", servingProvider(providerCfg(providerID, catwalk.TypeCompletions), model("")), providerID)
		}
	})

	t.Run("generic providers are selected by the model's wire type", func(t *testing.T) {
		t.Parallel()
		for wireType, expected := range map[catwalk.Type]string{
			catwalk.TypeResponses:   "openai",
			catwalk.TypeMessages:    "anthropic",
			catwalk.TypeCompletions: "openai-compat",
		} {
			require.Equal(t, expected, servingProvider(providerCfg("custom", ""), model(wireType)), wireType)
			require.Equal(t, expected, servingProvider(providerCfg("custom", wireType), model("")), wireType)
			for _, gateway := range []catwalk.InferenceProvider{
				catwalk.InferenceProviderCopilot,
				catwalk.InferenceProviderOpenCodeGo,
				catwalk.InferenceProviderOpenCodeZen,
			} {
				require.Equal(t, expected, servingProvider(providerCfg(string(gateway), catwalk.TypeCompletions), model(wireType)), gateway)
			}
		}
	})

	t.Run("custom provider types are passed through", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "litellm", servingProvider(providerCfg("litellm", "litellm"), model("")))
	})
}

func TestBuildProviderAllowsMultipleCompletionsProviders(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	providerCfg := config.ProviderConfig{
		ID:      "custom",
		BaseURL: "http://test.b32.i2p/v1",
		Type:    catwalk.TypeCompletions,
	}
	coord := newTestCoordinator(t, env, "custom", providerCfg)
	model := config.SelectedModel{Model: "some-model", Provider: "custom"}

	provider, err := coord.buildProvider(providerCfg, model, false, catwalk.TypeCompletions)
	require.NoError(t, err)
	require.NotNil(t, provider)

	for _, id := range []string{"opencode-zen", "openai", "hyper"} {
		cfg := providerCfg
		cfg.ID = id
		selected := model
		selected.Provider = id
		provider, err := coord.buildProvider(cfg, selected, false, catwalk.TypeCompletions)
		require.NoError(t, err)
		require.NotNil(t, provider)
	}

	otherModel := model
	otherModel.Provider = "openai"
	_, err = coord.buildProvider(providerCfg, otherModel, false, catwalk.TypeCompletions)
	require.ErrorContains(t, err, "does not match")

	for _, wireType := range []catwalk.Type{catwalk.TypeResponses, catwalk.TypeMessages, "unknown"} {
		_, err := coord.buildProvider(providerCfg, model, false, wireType)
		require.ErrorContains(t, err, "completions API")
	}

	providerCfg.Type = catwalk.TypeMessages
	_, err = coord.buildProvider(providerCfg, model, false, catwalk.TypeCompletions)
	require.ErrorContains(t, err, "completions API")
}

func TestBuildProviderRejectsInvalidResolvedSettings(t *testing.T) {
	t.Parallel()
	env := testEnv(t)
	coord := newTestCoordinator(t, env, "custom", config.ProviderConfig{})
	model := config.SelectedModel{Model: "some-model", Provider: "custom"}

	for _, tt := range []struct {
		name    string
		baseURL string
		apiKey  string
		want    string
	}{
		{name: "empty URL", want: "base URL must be an HTTP or HTTPS URL with a host"},
		{name: "missing host", baseURL: "https:///v1", want: "base URL must be an HTTP or HTTPS URL with a host"},
		{name: "unsupported scheme", baseURL: "file://localhost/v1", want: "base URL must be an HTTP or HTTPS URL with a host"},
		{name: "URL resolution error", baseURL: "$", want: "resolving custom provider base URL"},
		{name: "key resolution error", baseURL: "http://test.b32.i2p/v1", apiKey: "$", want: "resolving custom provider API key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProviderConfig{ID: "custom", BaseURL: tt.baseURL, APIKey: tt.apiKey}
			_, err := coord.buildProvider(cfg, model, false, catwalk.TypeCompletions)
			require.ErrorContains(t, err, tt.want)
		})
	}
}
