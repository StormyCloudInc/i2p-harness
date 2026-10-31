package model

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/catwalk/v3/pkg/catwalk"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/discover"
	"github.com/charmbracelet/crush/internal/i2p"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/dialog"
)

type providerAddedMsg struct {
	id            string
	connectedHost string
	err           error
}

func (m *UI) addProviderCmd(action dialog.ActionAddProvider) tea.Cmd {
	return func() tea.Msg {
		result := providerAddedMsg{id: action.ID}
		if _, exists := m.com.Config().Providers.Get(action.ID); exists {
			result.err = fmt.Errorf("provider %q already exists", action.ID)
			return result
		}
		var models []catwalk.Model
		if action.Model != "" {
			models = []catwalk.Model{{ID: action.Model, Name: action.Model}}
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			var err error
			models, err = discover.DiscoverModels(ctx, discover.Config{ID: action.ID, BaseURL: action.BaseURL, APIKey: action.APIKey}, config.IdentityResolver())
			if err != nil {
				// URL errors can include query parameters or userinfo. Report
				// the underlying cause without exposing the endpoint URL.
				var urlErr *url.Error
				if errors.As(err, &urlErr) {
					err = urlErr.Err
				}
				result.err = fmt.Errorf("could not discover models: %w; check the endpoint or enter a model ID", err)
				return result
			}
			if len(models) == 0 {
				result.err = fmt.Errorf("no models found; check the endpoint or enter a model ID")
				return result
			}
			if u, err := url.Parse(action.BaseURL); err == nil && i2p.IsI2PHost(u.Hostname()) {
				result.connectedHost = strings.TrimSuffix(u.Hostname(), ".")
			}
		}
		provider := config.ProviderConfig{
			ID: action.ID, Name: action.ID, Type: catwalk.TypeCompletions,
			BaseURL: common.LiteralProviderValue(action.BaseURL), APIKey: common.LiteralProviderValue(action.APIKey), Models: models,
		}
		if err := m.com.Workspace.SetConfigField(config.ScopeGlobal, "providers."+action.ID, provider); err != nil {
			result.err = fmt.Errorf("saving provider: %w", err)
			return result
		}
		if _, ok := m.com.Config().Providers.Get(action.ID); !ok {
			result.err = fmt.Errorf("provider was saved but could not be loaded; check the configuration")
		}
		return result
	}
}
