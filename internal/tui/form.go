package tui

import (
	"strconv"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/glattziehen/lazyfwd/internal/workspace"
)

type formKind int

const (
	formAddHost formKind = iota
	formAddForward
	formEditForward
)

// form is a modal list of text fields. Its submit handling lives in Model,
// which owns the workspace the values change.
type form struct {
	kind      formKind
	title     string
	direction workspace.Direction
	forwardID workspace.ForwardID
	labels    []string
	inputs    []textinput.Model
	focus     int
}

func newField(placeholder, value string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.SetValue(value)
	input.Prompt = ""
	input.CharLimit = 256
	input.Width = 40
	input.PlaceholderStyle = styleMuted
	return input
}

func newHostForm() *form {
	return newForm(&form{
		kind:   formAddHost,
		title:  "Add host",
		labels: []string{"ssh destination"},
		inputs: []textinput.Model{newField("user@box, box, or ssh://box:2222", "")},
	})
}

func newForwardForm(direction workspace.Direction) *form {
	f := &form{kind: formAddForward, direction: direction}
	if direction == workspace.Local {
		f.title = "Add local forward (-L): listens on the local end"
		f.labels = []string{"remote end(s)", "local end"}
		f.inputs = []textinput.Model{newField("4966-4968, db:5432, or /run/app.sock", ""), newField("auto: target port or nearest free", "")}
	} else {
		f.title = "Add remote forward (-R): listens on the remote end"
		f.labels = []string{"local end(s)", "remote end"}
		f.inputs = []textinput.Model{newField("3000, host:3000, or /run/app.sock", ""), newField("auto: same as target port", "")}
	}
	return newForm(f)
}

func newEditForm(forward workspace.Forward) *form {
	f := newForwardForm(forward.Direction)
	f.kind = formEditForward
	f.forwardID = forward.ID
	f.title = "Edit forward"
	f.labels[0] = "remote end"
	if forward.Direction == workspace.Remote {
		f.labels[0] = "local end"
	}
	f.inputs[0].SetValue(endText(forward.Target))
	f.inputs[1].SetValue(endText(forward.Bind))
	return f
}

// endText writes an end the way the forms read it back, as short as
// possible: a port alone when the host is the default, so editing the port
// is one word to replace.
func endText(end workspace.End) string {
	if end.IsSocket() {
		return end.Path
	}
	if end.Host == workspace.DefaultTargetHost {
		return strconv.Itoa(end.Port)
	}
	return end.String()
}

func newForm(f *form) *form {
	f.inputs[0].Focus()
	return f
}

func (f *form) value(i int) string {
	return f.inputs[i].Value()
}

// update moves focus on tab and up/down and passes other keys to the focused field.
func (f *form) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab", "down":
		f.setFocus((f.focus + 1) % len(f.inputs))
		return nil
	case "shift+tab", "up":
		f.setFocus((f.focus + len(f.inputs) - 1) % len(f.inputs))
		return nil
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}

func (f *form) setFocus(i int) {
	f.inputs[f.focus].Blur()
	f.focus = i
	f.inputs[f.focus].Focus()
}
