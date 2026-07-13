package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/SarthakShrivastav-a/hoppr/internal/connect"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
	"github.com/SarthakShrivastav-a/hoppr/internal/sshconfig"
)

// Field indices for the host form.
const (
	fAlias = iota
	fHostName
	fUser
	fPort
	fIdentity
	fProxyJump
	fTags
	fNote
	fCount
)

var fieldLabels = [fCount]string{
	fAlias:     "Alias",
	fHostName:  "HostName",
	fUser:      "User",
	fPort:      "Port",
	fIdentity:  "IdentityFile",
	fProxyJump: "ProxyJump",
	fTags:      "Tags",
	fNote:      "Note",
}

var fieldHints = [fCount]string{
	fAlias:     "what you type after ssh",
	fHostName:  "hostname or IP",
	fUser:      "leave blank for your local username",
	fPort:      "blank means 22",
	fIdentity:  "path to a private key",
	fProxyJump: "bastion to hop through, if any",
	fTags:      "comma separated",
	fNote:      "free text",
}

// form backs the add, edit, and import screens.
type form struct {
	inputs  []textinput.Model
	focus   int
	editing *host.Host // nil when adding
	err     string
}

func newForm(n int) *form {
	f := &form{inputs: make([]textinput.Model, n)}
	for i := range f.inputs {
		ti := textinput.New()
		ti.CharLimit = 512
		ti.Prompt = ""
		f.inputs[i] = ti
	}
	return f
}

func newAddForm() *form {
	f := newForm(fCount)
	for i := range f.inputs {
		f.inputs[i].Placeholder = fieldHints[i]
	}
	f.inputs[fAlias].Focus()
	return f
}

func newEditForm(h *host.Host) *form {
	f := newAddForm()
	f.editing = h

	f.inputs[fAlias].SetValue(h.Alias)
	f.inputs[fHostName].SetValue(h.HostName)
	f.inputs[fUser].SetValue(h.User)
	if h.Port != 0 {
		f.inputs[fPort].SetValue(strconv.Itoa(h.Port))
	}
	f.inputs[fIdentity].SetValue(strings.Join(h.IdentityFiles, ", "))
	f.inputs[fProxyJump].SetValue(h.ProxyJump)
	f.inputs[fTags].SetValue(strings.Join(h.Meta.Tags, ", "))
	f.inputs[fNote].SetValue(h.Meta.Note)

	return f
}

// newImportForm is a single field: the pasted command.
func newImportForm() *form {
	f := newForm(1)
	f.inputs[0].Placeholder = `ssh -i dev.pem ec2-user@dev-api.example.com`
	f.inputs[0].Focus()
	return f
}

func textinputBlink() tea.Cmd { return textinput.Blink }

// updateForm drives the add/edit screen.
func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := defaultKeys

	switch {
	case key.Matches(msg, k.Back):
		m.view = viewList
		m.form = nil
		return m, nil

	case msg.Type == tea.KeyTab, msg.Type == tea.KeyDown:
		m.form.next(1)
		return m, nil
	case msg.Type == tea.KeyShiftTab, msg.Type == tea.KeyUp:
		m.form.next(-1)
		return m, nil

	case msg.Type == tea.KeyEnter:
		return m.submitForm()
	}

	var cmd tea.Cmd
	m.form.inputs[m.form.focus], cmd = m.form.inputs[m.form.focus].Update(msg)
	return m, cmd
}

// updateImport drives the paste-a-command screen.
func (m Model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.view = viewList
		m.form = nil
		return m, nil

	case tea.KeyEnter:
		raw := strings.TrimSpace(m.form.inputs[0].Value())
		if raw == "" {
			return m, nil
		}

		spec, err := connect.ParseCommand(raw)
		if err != nil {
			m.form.err = err.Error()
			return m, nil
		}

		// Hand the parsed command to the normal form rather than writing it
		// straight out. The alias we guessed is a guess, and the user should
		// see and correct every field before it lands in their config.
		f := newAddForm()
		f.inputs[fAlias].SetValue(spec.Alias)
		f.inputs[fHostName].SetValue(spec.HostName)
		f.inputs[fUser].SetValue(spec.User)
		if spec.Port != 0 {
			f.inputs[fPort].SetValue(strconv.Itoa(spec.Port))
		}
		f.inputs[fIdentity].SetValue(strings.Join(spec.IdentityFiles, ", "))
		f.inputs[fProxyJump].SetValue(spec.ProxyJump)

		m.form = f
		m.view = viewForm
		return m, textinputBlink()
	}

	var cmd tea.Cmd
	m.form.err = ""
	m.form.inputs[0], cmd = m.form.inputs[0].Update(msg)
	return m, cmd
}

// submitForm validates and writes the host.
func (m Model) submitForm() (tea.Model, tea.Cmd) {
	f := m.form

	alias := strings.TrimSpace(f.value(fAlias))
	hostname := strings.TrimSpace(f.value(fHostName))

	if alias == "" {
		f.err = "an alias is required — it is what you type after ssh"
		return m, nil
	}
	if host.IsPattern(alias) {
		f.err = "an alias cannot contain * ? or a leading ! — those make it a pattern, not a host"
		return m, nil
	}
	if hostname == "" {
		f.err = "a hostname or IP is required"
		return m, nil
	}

	port := 0
	if p := strings.TrimSpace(f.value(fPort)); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			f.err = fmt.Sprintf("%q is not a valid port", p)
			return m, nil
		}
		port = n
	}

	// Locate or create the stanza.
	var (
		block *sshconfig.Block
		file  *sshconfig.File
	)
	if f.editing != nil {
		block, file = f.editing.Block(), f.editing.ConfigFile()
		if alias != f.editing.Alias {
			// Renaming: keep any secondary aliases the stanza already declared.
			rest := f.editing.Aliases[1:]
			block.SetPatterns(append([]string{alias}, rest...))
		}
	} else {
		if existing, _ := m.tree.FindHost(alias); existing.Block != nil {
			f.err = fmt.Sprintf("host %q already exists", alias)
			return m, nil
		}
		file = m.tree.Root
		block = file.AddHost(alias)
	}

	block.Set("HostName", hostname)
	setOrUnset(block, "User", strings.TrimSpace(f.value(fUser)))
	setOrUnset(block, "ProxyJump", strings.TrimSpace(f.value(fProxyJump)))

	if port != 0 {
		block.Set("Port", strconv.Itoa(port))
	} else {
		block.Unset("Port")
	}

	idents := splitList(f.value(fIdentity))
	if len(idents) > 0 {
		// Resolve a bare or relative key name to where the key actually is, and
		// store it in portable tilde form.
		for i, p := range idents {
			idents[i] = connect.ResolveKeyPath(p)
		}
		block.SetAll("IdentityFile", idents)
		// A host that names a key should use only that key; see doctor.
		block.Set("IdentitiesOnly", "yes")
	} else {
		block.Unset("IdentityFile")
		block.Unset("IdentitiesOnly")
	}

	block.SetMeta(sshconfig.Meta{
		Tags: splitList(f.value(fTags)),
		Note: strings.TrimSpace(f.value(fNote)),
	})

	if m.readOnly {
		return m, flashError(errReadOnly)
	}
	if err := file.Save(); err != nil {
		f.err = err.Error()
		return m, nil
	}

	m.view = viewList
	m.form = nil
	return m, tea.Batch(
		reloadCmd(m.configPath),
		flash("saved "+alias),
	)
}

// setOrUnset writes a keyword, or removes it when the field was cleared.
//
// The removal half matters: clearing the User field in the form has to actually
// delete the User line, not leave `User ""` behind.
func setOrUnset(b *sshconfig.Block, key, value string) {
	if value == "" {
		b.Unset(key)
		return
	}
	b.Set(key, value)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (f *form) value(i int) string {
	if i >= len(f.inputs) {
		return ""
	}
	return f.inputs[i].Value()
}

// next moves focus, wrapping at both ends.
func (f *form) next(delta int) {
	f.err = ""
	f.inputs[f.focus].Blur()

	n := len(f.inputs)
	f.focus = (f.focus + delta + n) % n

	f.inputs[f.focus].Focus()
}
