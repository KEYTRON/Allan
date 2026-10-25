package tui

import (
	"fmt"
	"github.com/keytron/allan/agent/internal/i18n"
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
	Free    bool   // passes the "free only" filter
	Section int    // index into picker.sections
	// Folded items are hidden from the "all" view until the user searches or
	// opens their section: catalogs of hundreds of models drown the rest.
	Folded bool
	Value  any
}

// picker is a modal list: arrows move, typing filters, Enter picks, Esc closes.
// While it is open the input box acts as the filter field. With sections set,
// Tab steps through them and Ctrl+F keeps only free items.
type picker struct {
	title    string
	items    []pickItem
	visible  []int // indexes into items that pass the filter
	selected int   // index into visible
	onPick   func(pickItem) tea.Cmd

	sections []string // names of pickItem.Section values; nil — no tabs
	section  int      // -1 — all sections
	freeOnly bool
	query    string
	hidden   []int // per section: folded items left out of the current view
}

const pickerRows = 12

func newPicker(title string, items []pickItem, onPick func(pickItem) tea.Cmd) *picker {
	p := &picker{title: title, items: items, onPick: onPick, section: -1}
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
	p.query = q
	words := strings.Fields(strings.ToLower(q))
	keep := -1 // item under the cursor stays selected if it still matches
	if p.selected < len(p.visible) {
		keep = p.visible[p.selected]
	}
	p.visible = p.visible[:0]
	fold := p.section == -1 && len(words) == 0
	p.hidden = make([]int, len(p.sections))
	for i, it := range p.items {
		if p.section >= 0 && it.Section != p.section || p.freeOnly && !it.Free {
			continue
		}
		if fold && it.Folded && !it.Current {
			if it.Section < len(p.hidden) {
				p.hidden[it.Section]++
			}
			continue
		}
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

// setSections turns on tabs and folding; the cursor stays on the current item.
func (p *picker) setSections(names []string) {
	p.sections = names
	p.filter(p.query)
	for i, idx := range p.visible {
		if p.items[idx].Current {
			p.selected = i
		}
	}
}

// counts reports how many items each section has under the current
// free-only setting, ignoring the text filter.
func (p *picker) counts() []int {
	n := make([]int, len(p.sections))
	for _, it := range p.items {
		if it.Section < len(n) && (!p.freeOnly || it.Free) {
			n[it.Section]++
		}
	}
	return n
}

// cycleSection moves to the next (or previous) non-empty section; "all" is
// one of the stops.
func (p *picker) cycleSection(delta int) {
	n := p.counts()
	for range len(n) + 1 {
		p.section += delta
		if p.section >= len(n) {
			p.section = -1
		} else if p.section < -1 {
			p.section = len(n) - 1
		}
		if p.section == -1 || n[p.section] > 0 {
			break
		}
	}
	p.filter(p.query)
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
	case tea.KeyUp:
		p.move(-1)
	case tea.KeyDown:
		p.move(1)
	case tea.KeyTab, tea.KeyShiftTab:
		delta := 1
		if msg.Type == tea.KeyShiftTab {
			delta = -1
		}
		if p.sections == nil {
			p.move(delta)
		} else {
			p.cycleSection(delta)
		}
	case tea.KeyCtrlF:
		if p.sections == nil {
			return nil, false
		}
		p.freeOnly = !p.freeOnly
		p.filter(p.query)
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

	lines := []string{StyleBrand.Render(p.title)}
	if p.sections != nil {
		lines = append(lines, p.renderTabs(inner))
	}
	lines = append(lines, "")
	lastGroup := "" // the first row always shows its group, even mid-list
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
		lines = append(lines, StyleMuted.Render(i18n.S("  ничего не найдено")))
	}
	if folded := p.foldedNote(); folded != "" {
		lines = append(lines, StyleDim.Render(truncate(folded, inner)))
	}
	hint := i18n.S("%d/%d · ↑↓ выбор · Enter выбрать · Esc отмена · печатайте для поиска")
	if p.sections != nil {
		hint = i18n.S("%d/%d · ↑↓/колесо выбор · Tab раздел · Ctrl+F бесплатные · Enter выбрать · Esc отмена · печатайте для поиска")
	}
	lines = append(lines, "", StyleDim.Render(truncate(fmt.Sprintf(hint, minInt(p.selected+1, len(p.visible)), len(p.visible)), inner)))
	return StylePopup.Width(width).Render(strings.Join(lines, "\n"))
}

// renderTabs is the row of sections with their sizes; the active one is lit.
func (p *picker) renderTabs(width int) string {
	n := p.counts()
	total := 0
	for _, c := range n {
		total += c
	}
	tab := func(name string, count int, on bool) string {
		s := fmt.Sprintf("%s %d", name, count)
		if on {
			return StylePopupSelected.Render("[" + s + "]")
		}
		return StyleMuted.Render(" " + s + " ")
	}
	parts := []string{tab(i18n.S("Все"), total, p.section == -1)}
	for i, name := range p.sections {
		if n[i] > 0 {
			parts = append(parts, tab(name, n[i], p.section == i))
		}
	}
	row := strings.Join(parts, " ")
	if p.freeOnly {
		row += "  " + StyleOK.Render(i18n.S("только бесплатные"))
	}
	if lipgloss.Width(row) > width {
		// narrow terminal: keep the active tab readable rather than the whole row
		row = truncate(row, width)
	}
	return row
}

// foldedNote says how much the "all" view leaves out and how to see it.
func (p *picker) foldedNote() string {
	parts := []string{}
	for i, n := range p.hidden {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", p.sections[i], n))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf(i18n.S("  … ещё %s — Tab на раздел или поиск"), strings.Join(parts, ", "))
}
