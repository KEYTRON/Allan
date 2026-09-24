package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickItem is one row of an interactive list.
type pickItem struct {
	Label   string // main text, also what the filter matches
	Group   string // section header shown above the first item of a group
	Detail  string // dim text on the right
	Current bool   // marks the active choice with ●
	Dim     bool   // shown greyed out (e.g. embedding models)
	Value   any
}

// picker is a modal list: arrows move, typing filters, Enter picks, Esc closes.
// While it is open the input box acts as the filter field.
type picker struct {
	title    string
	items    []pickItem
	visible  []int // indexes into items that pass the filter
	selected int   // index into visible
	onPick   func(pickItem) tea.Cmd
}

const pickerRows = 12

func newPicker(title string, items []pickItem, onPick func(pickItem) tea.Cmd) *picker {
	p := &picker{title: title, items: items, onPick: onPick}
	p.filter("")
	for i, idx := range p.visible {
		if p.items[idx].Current {
			p.selected = i
		}
	}
	return p
}

// filter keeps items whose label, group or detail contain every word of q.
func (p *picker) filter(q string) {
	words := strings.Fields(strings.ToLower(q))
	keep := -1 // item under the cursor stays selected if it still matches
	if p.selected < len(p.visible) {
		keep = p.visible[p.selected]
	}
	p.visible = p.visible[:0]
	for i, it := range p.items {
		hay := strings.ToLower(it.Label + " " + it.Group + " " + it.Detail)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			p.visible = append(p.visible, i)
		}
	}
	p.selected = 0
	for i, idx := range p.visible {
		if idx == keep {
			p.selected = i
		}
	}
}

func (p *picker) move(delta int) {
	if len(p.visible) == 0 {
		return
	}
	p.selected = (p.selected + delta + len(p.visible)) % len(p.visible)
}

func (p *picker) current() (pickItem, bool) {
	if len(p.visible) == 0 {
		return pickItem{}, false
	}
	return p.items[p.visible[p.selected]], true
}

// openPicker shows a picker and turns the input box into its filter field.
func (m *Model) openPicker(p *picker) {
	m.picker = p
	m.popupOpen = false
	m.textarea.SetValue("")
	m.textarea.Placeholder = "Enter — выбрать, печатайте для поиска" // ASCII first, see inputPlaceholder
	m.recalcLayout()
}

func (m *Model) closePicker() {
	m.picker = nil
	m.textarea.SetValue("")
	m.textarea.Placeholder = inputPlaceholder
	m.recalcLayout()
}

// pickerKey handles keys while the picker is open. The bool reports whether
// the key was consumed; other keys fall through to the input (the filter).
func (m *Model) pickerKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	p := m.picker
	switch msg.Type {
	case tea.KeyEsc:
		m.closePicker()
		return nil, true
	case tea.KeyUp, tea.KeyShiftTab:
		p.move(-1)
	case tea.KeyDown, tea.KeyTab:
		p.move(1)
	case tea.KeyPgUp:
		p.move(-pickerRows)
	case tea.KeyPgDown:
		p.move(pickerRows)
	case tea.KeyHome:
		p.selected = 0
	case tea.KeyEnd:
		p.selected = maxInt(0, len(p.visible)-1)
	case tea.KeyEnter:
		it, ok := p.current()
		m.closePicker()
		if !ok {
			return nil, true
		}
		return p.onPick(it), true
	default:
		return nil, false
	}
	m.recalcLayout()
	return nil, true
}

func (m *Model) renderPicker() string {
	p := m.picker
	width := maxInt(20, m.width-2)
	inner := width - 4

	start := 0
	if p.selected >= pickerRows {
		start = p.selected - pickerRows + 1
	}
	end := minInt(len(p.visible), start+pickerRows)

	labelW := 0
	for _, idx := range p.visible[start:end] {
		labelW = maxInt(labelW, lipgloss.Width(p.items[idx].Label))
	}
	labelW = minInt(labelW, maxInt(10, inner*2/3))

	lines := []string{StyleBrand.Render(p.title), ""}
	lastGroup := ""
	if start > 0 {
		lastGroup = p.items[p.visible[start-1]].Group
	}
	for i := start; i < end; i++ {
		it := p.items[p.visible[i]]
		if it.Group != "" && it.Group != lastGroup {
			lines = append(lines, StyleMuted.Render(it.Group))
			lastGroup = it.Group
		}
		mark := "  "
		if it.Current {
			mark = StyleOK.Render("● ")
		}
		label := padRight(truncate(it.Label, labelW), labelW)
		detail := truncate(it.Detail, maxInt(0, inner-labelW-8))
		switch {
		case i == p.selected:
			lines = append(lines, StylePopupSelected.Render("› ")+mark+StylePopupSelected.Render(label)+"  "+StyleMuted.Render(detail))
		case it.Dim:
			lines = append(lines, "  "+mark+StyleDim.Render(label)+"  "+StyleDim.Render(detail))
		default:
			lines = append(lines, "  "+mark+StyleText.Render(label)+"  "+StyleDim.Render(detail))
		}
	}
	if len(p.visible) == 0 {
		lines = append(lines, StyleMuted.Render("  ничего не найдено"))
	}
	lines = append(lines, "", StyleDim.Render(fmt.Sprintf("%d/%d · ↑↓ выбор · Enter выбрать · Esc отмена · печатайте для поиска",
		minInt(p.selected+1, len(p.visible)), len(p.visible))))
	return StylePopup.Width(width).Render(strings.Join(lines, "\n"))
}
