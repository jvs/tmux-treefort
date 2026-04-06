package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const jumpTimeout = 500 * time.Millisecond

type ItemKind int

const (
	KindSession ItemKind = iota
	KindWindow
)

type Item struct {
	Kind     ItemKind
	Session  *Session
	Window   *Window // nil for session items
	Enabled  bool
	JumpCode int
	IsLast   bool   // last window in its session, for guide rendering
	Name     string // plain text name, used for cursor-row rendering
	Label    string // name with ANSI fuzzy markup and number prefix, used for non-cursor rows
}

type HistoryEntry struct {
	Kind      ItemKind
	SessionID string
	WindowID  string // empty for session entries
}

type ModalAction int

const (
	ActionNone ModalAction = iota
	ActionAdd
	ActionRename
	ActionDelete
)

type Model struct {
	sessions []Session
	items    []Item
	cursor   int

	searchTerm string
	searchMode bool

	inputMode   bool
	inputPrompt string
	inputValue  []rune

	showNumbers bool
	showGuides  bool

	focusSession string // session ID, or "" for all

	modal       tea.Model
	modalAction ModalAction

	initialSessID string
	initialWinID  string

	jumpBuffer  string
	jumpUpdated time.Time

	commandFile   string
	returnCommand string
	switchCommand string
	visitCommand  string

	history []HistoryEntry

	width  int
	height int
}

var (
	matchStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	numberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func newModel(initialSessID, initialWinID, commandFile, returnCommand, switchCommand, visitCommand string, searchMode bool) Model {
	m := Model{
		showGuides:    true,
		initialSessID: initialSessID,
		initialWinID:  initialWinID,
		commandFile:   commandFile,
		returnCommand: returnCommand,
		switchCommand: switchCommand,
		visitCommand:  visitCommand,
		searchMode:    searchMode,
		width:         80,
		height:        24,
	}
	m.sessions, _ = loadSessions()
	m.items = buildItems(m)

	// Position cursor on the current window.
	for i, item := range m.items {
		if item.Kind == KindWindow && item.Window.ID == initialWinID {
			m.cursor = i
			break
		}
	}
	m.recordHistory()
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case ModalDoneMsg:
		return m.handleModalDone(msg)

	case tea.KeyMsg:
		// Delegate to active modal.
		if m.modal != nil {
			var cmd tea.Cmd
			m.modal, cmd = m.modal.Update(msg)
			return m, cmd
		}

		if m.inputMode {
			return m.handleInputKey(msg)
		}
		if m.searchMode {
			return m.handleSearchKey(msg)
		}
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchTerm = ""
		m.items = buildItems(m)
	case "enter":
		m.searchMode = false
	case "backspace", "ctrl+h":
		if len(m.searchTerm) > 0 {
			runes := []rune(m.searchTerm)
			m.searchTerm = string(runes[:len(runes)-1])
			m.items = buildItems(m)
			m.moveCursorToFirstEnabled()
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.searchTerm += string(msg.Runes)
			m.items = buildItems(m)
			m.moveCursorToFirstEnabled()
		}
	}
	return m, nil
}

func (m Model) handleInputKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = false
		m.inputValue = nil
		m.modalAction = ActionNone
	case "enter":
		value := string(m.inputValue)
		m.inputMode = false
		m.inputValue = nil
		return m.handleModalDone(ModalDoneMsg{Value: &value})
	case "backspace", "ctrl+h":
		if len(m.inputValue) > 0 {
			m.inputValue = m.inputValue[:len(m.inputValue)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.inputValue = append(m.inputValue, msg.Runes...)
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.cursor++
		m.clampCursor()
		m.recordHistory()
		return m, m.switchToCurrentCmd()

	case "k", "up":
		m.cursor--
		m.clampCursor()
		m.recordHistory()
		return m, m.switchToCurrentCmd()

	case "enter":
		if m.visitCommand != "" && m.commandFile != "" {
			os.WriteFile(m.commandFile, []byte(m.visitCommand+"\n"), 0644)
		}
		return m, tea.Quit

	case "esc", "alt+o":
		target := m.initialSessID + ":" + m.initialWinID
		tmuxRun("switch-client", "-t", target)
		return m, tea.Quit

	case "alt+u":
		if m.switchCommand != "" && m.commandFile != "" {
			os.WriteFile(m.commandFile, []byte(m.switchCommand+"\n"), 0644)
		}
		return m, tea.Quit

	case "/":
		m.searchMode = true
		return m, nil

	case "n":
		m.showNumbers = !m.showNumbers
		m.items = buildItems(m)
		return m, nil

	case "g":
		m.showGuides = !m.showGuides
		return m, nil

	case "s":
		if m.focusSession == "" {
			if item := m.currentItem(); item != nil {
				m.focusSession = item.Session.ID
			}
		} else {
			m.focusSession = ""
		}
		m.items = buildItems(m)
		m.clampCursor()
		return m, nil

	case "a":
		m.inputMode = true
		m.inputPrompt = "Name"
		m.inputValue = nil
		m.modalAction = ActionAdd
		return m, nil

	case "r":
		m.inputMode = true
		m.inputPrompt = "Rename"
		m.inputValue = nil
		m.modalAction = ActionRename
		return m, nil

	case "d":
		if item := m.currentItem(); item != nil {
			kind := "session"
			name := item.Session.Name
			if item.Kind == KindWindow {
				kind = "window"
				name = item.Window.Name
			}
			m.modal = newConfirmModal(fmt.Sprintf("Delete %s %q?", kind, name))
			m.modalAction = ActionDelete
		}
		return m, nil
	}

	// Number-jump mode.
	if m.showNumbers {
		ch := msg.String()
		if len(ch) == 1 && ch >= "0" && ch <= "9" {
			now := time.Now()
			if !m.jumpUpdated.IsZero() && now.Sub(m.jumpUpdated) < jumpTimeout {
				m.jumpBuffer += ch
			} else {
				m.jumpBuffer = ch
			}
			m.jumpUpdated = now
			if code, err := strconv.Atoi(m.jumpBuffer); err == nil {
				for i, item := range m.items {
					if item.JumpCode == code {
						m.cursor = i
						m.recordHistory()
						return m, m.switchToCurrentCmd()
					}
				}
			}
		}
	}

	return m, nil
}

func (m Model) handleModalDone(msg ModalDoneMsg) (Model, tea.Cmd) {
	m.modal = nil
	action := m.modalAction
	m.modalAction = ActionNone

	if msg.Value == nil {
		return m, nil // cancelled
	}

	switch action {
	case ActionAdd:
		return m.handleAdd(*msg.Value)
	case ActionRename:
		return m.handleRename(*msg.Value)
	case ActionDelete:
		return m.handleDelete()
	}
	return m, nil
}

func (m Model) handleAdd(name string) (Model, tea.Cmd) {
	item := m.currentItem()
	if item == nil || name == "" {
		return m, nil
	}

	position := "a" // after current window
	targetID := item.Session.ID
	if item.Kind == KindWindow {
		targetID = item.Window.ID
	} else {
		position = "b" // before first window (beginning of session)
	}

	if m.commandFile != "" {
		content := fmt.Sprintf("tmux new-window -%s -t '%s' -n '%s' -c '#{pane_current_path}'\n",
			position, targetID, name)
		if m.returnCommand != "" {
			content += m.returnCommand + "\n"
		}
		os.WriteFile(m.commandFile, []byte(content), 0644)
		return m, tea.Quit
	}

	// No command file: run directly.
	exec.Command("tmux", "new-window", "-"+position, "-t", targetID, "-n", name,
		"-c", "#{pane_current_path}").Run()
	m.refresh()
	return m, nil
}

func (m Model) handleRename(name string) (Model, tea.Cmd) {
	item := m.currentItem()
	if item == nil || name == "" {
		return m, nil
	}

	var targetID string
	if item.Kind == KindSession {
		targetID = item.Session.ID
		tmuxRun("rename-session", "-t", targetID, name)
	} else {
		targetID = item.Window.ID
		tmuxRun("rename-window", "-t", targetID, name)
	}

	kind := item.Kind
	m.refresh()

	// Restore cursor to the renamed item.
	for i, it := range m.items {
		if kind == KindSession && it.Kind == KindSession && it.Session.ID == targetID {
			m.cursor = i
			break
		}
		if kind == KindWindow && it.Kind == KindWindow && it.Window.ID == targetID {
			m.cursor = i
			break
		}
	}
	return m, nil
}

func (m Model) handleDelete() (Model, tea.Cmd) {
	item := m.currentItem()
	if item == nil {
		return m, nil
	}

	// Find the next focus target from history before deleting.
	next := m.nextFromHistory(item)
	if next != nil {
		target := next.SessionID
		if next.WindowID != "" {
			target += ":" + next.WindowID
		}
		tmuxRun("switch-client", "-t", target)
	}

	if item.Kind == KindSession {
		tmuxRun("kill-session", "-t", item.Session.ID)
	} else {
		tmuxRun("kill-window", "-t", item.Window.ID)
	}

	m.refresh()

	// Move cursor to next target.
	if next != nil {
		for i, it := range m.items {
			if next.Kind == KindSession && it.Kind == KindSession && it.Session.ID == next.SessionID {
				m.cursor = i
				break
			}
			if next.Kind == KindWindow && it.Kind == KindWindow && it.Window.ID == next.WindowID {
				m.cursor = i
				break
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.modal != nil {
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.modal.View())
	}

	if len(m.items) == 0 {
		return "No tmux sessions found.\n"
	}

	var lines []string
	lines = append(lines, "")
	for i, item := range m.items {
		lines = append(lines, m.renderItem(i, item))
	}
	out := strings.Join(lines, "\n")

	// Always render the bottom bar so the view height is constant.
	// A variable-height view causes Bubble Tea's diff renderer to leave artifacts.
	var bar string
	switch {
	case m.searchMode:
		bar = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("/ ") +
			m.searchTerm + modalCursorStyle.Render("█")
	case m.inputMode:
		bar = dimStyle.Render(m.inputPrompt+": ") +
			string(m.inputValue) + modalCursorStyle.Render("█")
	}
	out += "\n" + bar

	return out
}

func (m Model) renderItem(i int, item Item) string {
	var prefix string
	if item.Kind == KindWindow {
		if m.showGuides {
			if item.IsLast {
				prefix = "   └─ "
			} else {
				prefix = "   ├─ "
			}
		} else {
			prefix = "   "
		}
	}

	if i == m.cursor {
		// Build cursor row from plain text so ANSI resets inside Label don't
		// break the background color mid-line.
		var numPrefix string
		if m.showNumbers {
			numPrefix = fmt.Sprintf("%d: ", item.JumpCode)
		}
		plain := "   " + prefix + numPrefix + item.Name
		return lipgloss.NewStyle().Background(lipgloss.Color("237")).Width(m.width).Render(plain)
	}
	return "   " + prefix + item.Label
}

// buildItems rebuilds the flat visible item list from current model state.
func buildItems(m Model) []Item {
	var items []Item
	jumpCode := 0

	for si := range m.sessions {
		sess := &m.sessions[si]

		if strings.HasPrefix(sess.Name, "__") {
			continue
		}
		if m.focusSession != "" && sess.ID != m.focusSession {
			continue
		}

		jumpCode++
		label, enabled := applyFuzzyMarkup(m.searchTerm, "", sess.Name)
		if m.showNumbers {
			label = numberStyle.Render(fmt.Sprintf("%d:", jumpCode)) + " " + label
		}
		items = append(items, Item{
			Kind: KindSession, Session: sess,
			Enabled: enabled, JumpCode: jumpCode,
			Name: sess.Name, Label: label,
		})

		for wi := range sess.Windows {
			win := &sess.Windows[wi]
			jumpCode++
			winLabel, winEnabled := applyFuzzyMarkup(m.searchTerm, sess.Name, win.Name)
			if m.showNumbers {
				winLabel = numberStyle.Render(fmt.Sprintf("%d:", jumpCode)) + " " + winLabel
			}
			items = append(items, Item{
				Kind: KindWindow, Session: sess, Window: win,
				Enabled: winEnabled, JumpCode: jumpCode,
				IsLast: wi == len(sess.Windows)-1,
				Name:   win.Name, Label: winLabel,
			})
		}
	}

	return items
}

// applyFuzzyMarkup returns the display label and whether the item matches the search.
// parentName is the session name for window items, empty for session items.
func applyFuzzyMarkup(searchTerm, parentName, name string) (string, bool) {
	if searchTerm == "" {
		return name, true
	}

	parentFilter, childFilter, hasSlash := strings.Cut(searchTerm, "/")
	if !hasSlash {
		childFilter = searchTerm
		parentFilter = ""
	}

	if parentName != "" && parentFilter != "" {
		if !fuzzyMatch(parentFilter, parentName) {
			return dimStyle.Render(name), false
		}
	}

	childFilter = strings.ToLower(strings.ReplaceAll(childFilter, " ", ""))
	if childFilter == "" {
		return name, true
	}

	filterRunes := []rune(childFilter)
	var b strings.Builder
	fi := 0

	for _, ch := range name {
		if fi < len(filterRunes) && unicode.ToLower(ch) == filterRunes[fi] {
			b.WriteString(matchStyle.Render(string(ch)))
			fi++
		} else {
			b.WriteString(string(ch))
		}
	}

	if fi == len(filterRunes) {
		return b.String(), true
	}
	return dimStyle.Render(name), false
}

func fuzzyMatch(term, name string) bool {
	if term == "" {
		return true
	}
	term = strings.ToLower(term)
	fi := 0
	for _, ch := range strings.ToLower(name) {
		if fi < len([]rune(term)) && ch == []rune(term)[fi] {
			fi++
		}
	}
	return fi == len([]rune(term))
}

func (m Model) currentItem() *Item {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.cursor]
}

func (m *Model) clampCursor() {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if len(m.items) > 0 && m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
}

func (m *Model) moveCursorToFirstEnabled() {
	for i, item := range m.items {
		if item.Enabled {
			m.cursor = i
			return
		}
	}
}

func (m *Model) recordHistory() {
	item := m.currentItem()
	if item == nil {
		return
	}
	entry := HistoryEntry{Kind: item.Kind, SessionID: item.Session.ID}
	if item.Kind == KindWindow {
		entry.WindowID = item.Window.ID
	}
	m.history = append(m.history, entry)
}

func (m Model) nextFromHistory(deleted *Item) *HistoryEntry {
	deletedSessID := deleted.Session.ID
	deletedWinID := ""
	if deleted.Kind == KindWindow {
		deletedWinID = deleted.Window.ID
	}

	for i := len(m.history) - 1; i >= 0; i-- {
		h := m.history[i]
		// Skip any entry that is the deleted item.
		if h.SessionID == deletedSessID && h.WindowID == deletedWinID {
			continue
		}
		// When deleting a session, skip all entries within it.
		if deleted.Kind == KindSession && h.SessionID == deletedSessID {
			continue
		}
		return &h
	}
	return nil
}

func (m Model) switchToCurrentCmd() tea.Cmd {
	item := m.currentItem()
	if item == nil {
		return nil
	}
	target := item.Session.ID
	if item.Kind == KindWindow {
		target = item.Session.ID + ":" + item.Window.ID
	}
	return func() tea.Msg {
		tmuxRun("switch-client", "-t", target)
		return nil
	}
}

func (m *Model) refresh() {
	m.sessions, _ = loadSessions()
	m.items = buildItems(*m)
	m.clampCursor()
}
