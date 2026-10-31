package dialog

import (
	"net/url"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/i2p"
	"github.com/charmbracelet/crush/internal/ui/common"
	uv "github.com/charmbracelet/ultraviolet"
)

// ProviderAddID identifies the custom provider setup dialog.
const ProviderAddID = "provider_add"

// ProviderAdd collects connection details without blocking the UI.
type ProviderAdd struct {
	com      *common.Common
	inputs   [4]textinput.Model
	step     int
	err      string
	saving   bool
	help     help.Model
	next     key.Binding
	previous key.Binding
}

var _ Dialog = (*ProviderAdd)(nil)

func NewProviderAdd(com *common.Common) *ProviderAdd {
	d := &ProviderAdd{com: com, help: help.New()}
	d.help.Styles = com.Styles.DialogHelpStyles()
	d.next = key.NewBinding(key.WithKeys("enter", "tab"), key.WithHelp("enter", "next"))
	d.previous = key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "back"))
	for i := range d.inputs {
		d.inputs[i] = textinput.New()
		d.inputs[i].SetVirtualCursor(false)
		d.inputs[i].SetStyles(com.Styles.TextInput)
	}
	d.inputs[0].Placeholder = "my-provider"
	d.inputs[1].Placeholder = "http://example.b32.i2p/v1"
	d.inputs[2].Placeholder = "Optional"
	d.inputs[2].EchoMode = textinput.EchoPassword
	d.inputs[3].Placeholder = "Optional; leave blank to discover models"
	d.inputs[0].Focus()
	return d
}

func (d *ProviderAdd) ID() string { return ProviderAddID }

func (d *ProviderAdd) SetError(err string) {
	d.saving = false
	d.err = err
}

func (d *ProviderAdd) HandleMsg(msg tea.Msg) Action {
	if d.saving {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, CloseKey):
			return ActionClose{}
		case key.Matches(msg, d.previous):
			if d.step > 0 {
				d.inputs[d.step].Blur()
				d.step--
				d.inputs[d.step].Focus()
			}
			d.err = ""
			return nil
		case key.Matches(msg, d.next):
			if err := d.validateStep(); err != "" {
				d.err = err
				return nil
			}
			d.err = ""
			if d.step < len(d.inputs)-1 {
				d.inputs[d.step].Blur()
				d.step++
				d.inputs[d.step].Focus()
				return nil
			}
			d.saving = true
			return ActionAddProvider{ID: strings.TrimSpace(d.inputs[0].Value()), BaseURL: strings.TrimSpace(d.inputs[1].Value()), APIKey: d.inputs[2].Value(), Model: strings.TrimSpace(d.inputs[3].Value())}
		}
	case tea.PasteMsg:
	default:
		return nil
	}
	var cmd tea.Cmd
	d.inputs[d.step], cmd = d.inputs[d.step].Update(msg)
	d.err = ""
	return ActionCmd{Cmd: cmd}
}

func (d *ProviderAdd) validateStep() string {
	switch d.step {
	case 0:
		id := strings.TrimSpace(d.inputs[0].Value())
		if err := config.ValidateCustomProviderID(id); err != nil {
			return err.Error()
		}
		if _, exists := d.com.Config().Providers.Get(id); exists {
			return "A provider with this ID already exists."
		}
	case 1:
		u, err := url.Parse(strings.TrimSpace(d.inputs[1].Value()))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return "Enter an HTTP or HTTPS URL with a host."
		}
		if !i2p.IsI2PHost(u.Hostname()) {
			return "Enter a URL with an .i2p host."
		}
	}
	return ""
}

func (d *ProviderAdd) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(65, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	innerWidth := max(0, width-t.Dialog.View.GetHorizontalFrameSize())
	d.inputs[d.step].SetWidth(dialogInputTextWidth(t, d.inputs[d.step], innerWidth))
	rc := NewRenderContext(t, width)
	rc.Title = "Add Provider"
	labels := []string{"Provider ID", "OpenAI-compatible base URL", "API key (optional)", "Model ID (optional)"}
	rc.AddPart(t.Dialog.PrimaryText.Render(labels[d.step]))
	rc.AddPart(t.Dialog.InputPrompt.Render(d.inputs[d.step].View()))
	if d.saving {
		message := "Saving provider..."
		if strings.TrimSpace(d.inputs[3].Value()) == "" {
			if u, err := url.Parse(strings.TrimSpace(d.inputs[1].Value())); err == nil && i2p.IsI2PHost(u.Hostname()) {
				message = "Dialing " + strings.TrimSuffix(u.Hostname(), ".") + "..."
			}
		}
		rc.AddPart(t.Dialog.SecondaryText.Width(innerWidth).Render(message))
	} else if d.err != "" {
		rc.AddPart(t.Dialog.TitleError.Width(innerWidth).Render(d.err))
	} else if d.step == 3 {
		rc.AddPart(t.Dialog.SecondaryText.Width(innerWidth).Render("Leave blank to discover models from the endpoint."))
	}
	rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)
	cur := InputCursor(t, d.inputs[d.step].Cursor())
	if cur != nil {
		cur.Y++ // The field label precedes the input.
	}
	DrawCenterCursor(scr, area, rc.Render(), cur)
	return cur
}

func (d *ProviderAdd) ShortHelp() []key.Binding {
	return []key.Binding{d.next, d.previous, CloseKey}
}

func (d *ProviderAdd) FullHelp() [][]key.Binding { return [][]key.Binding{d.ShortHelp()} }
