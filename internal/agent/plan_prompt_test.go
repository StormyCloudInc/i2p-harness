package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/crush/internal/agent/prompt"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/stretchr/testify/require"
)

// TestPlanPromptListsConfiguredTools verifies the plan system prompt
// advertises the tools the plan agent is actually configured with. The
// list is generated from the agent's AllowedTools rather than hard-coded,
// so the prompt can never promise a tool the user disabled.
func TestPlanPromptListsConfiguredTools(t *testing.T) {
	env := testEnv(t)

	// Minimal hermetic config so config.Init and the prompt build both
	// succeed without touching the user's real provider config.
	crushJSON := `{
  "options": {"disable_default_providers": true, "disable_provider_auto_update": true},
  "providers": {"custom": {"id": "custom", "name": "Mock", "type": "openai-compat",
    "base_url": "http://test.b32.i2p/v1", "api_key": "test-key",
    "models": [{"id": "mock-model", "name": "Mock", "context_window": 8192, "default_max_tokens": 128}]}},
  "models": {"large": {"provider": "custom", "model": "mock-model"},
             "small": {"provider": "custom", "model": "mock-model"}}
}`
	require.NoError(t, os.WriteFile(filepath.Join(env.workingDir, "crush.json"), []byte(crushJSON), 0o644))

	cfg, err := config.Init(t.Context(), env.workingDir, "", false)
	require.NoError(t, err)
	cfg.SetupAgents()

	p, err := planPrompt(prompt.WithWorkingDir(env.workingDir))
	require.NoError(t, err)

	systemPrompt, err := p.Build(context.Background(), "custom", "mock-model", cfg)
	require.NoError(t, err)
	require.Contains(t, systemPrompt,
		"Your available tools are: agent, lsp_symbols, lsp_definition, lsp_call_hierarchy, glob, grep, ls, question, sourcegraph, view.")

	// A tool the user disabled disappears from the advertised list.
	// (The word "question" still appears elsewhere in the prompt's rules,
	// so assert on the exact generated tools line instead.)
	cfg.Config().Options.DisabledTools = []string{"question"}
	cfg.SetupAgents()
	systemPrompt, err = p.Build(context.Background(), "custom", "mock-model", cfg)
	require.NoError(t, err)
	require.Contains(t, systemPrompt,
		"Your available tools are: agent, lsp_symbols, lsp_definition, lsp_call_hierarchy, glob, grep, ls, sourcegraph, view.")
	require.NotContains(t, systemPrompt, "ls, question,")
}
