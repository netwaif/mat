package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/netwaif/mat/internal/model"
	"github.com/netwaif/mat/internal/parser"
)

type mode int

const (
	modeMain mode = iota
	modeModal
	modeLogModal
)

// tickInterval is the auto-refresh polling period for the main view.
const tickInterval = 2 * time.Second

// Model is the bubbletea root model.
type Model struct {
	root        string
	taskName    string
	task        model.Task
	loaded      bool
	loadErr     string
	mode        mode
	modalItems  []model.TaskBrief
	modalCursor int
	logScroll   int // top-of-window offset for the log modal
	width       int
	height      int
	startupOpen bool
}

func NewModel(root, taskName string) Model {
	return Model{root: root, taskName: taskName}
}

func (m Model) WithStartupModal() Model {
	m.startupOpen = true
	return m
}

// --- messages ---

type loadedMsg struct {
	task model.Task
	err  error
}

type modalListMsg struct {
	items []model.TaskBrief
}

type tickMsg time.Time

// --- commands ---

func loadTaskCmd(root, name string) tea.Cmd {
	return func() tea.Msg {
		if name == "" {
			return loadedMsg{err: fmt.Errorf("no active task")}
		}
		t, err := parser.LoadTask(root, name)
		return loadedMsg{task: t, err: err}
	}
}

func loadModalCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return modalListMsg{items: parser.ListTasks(root)}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// --- bubbletea contract ---

func (m Model) Init() tea.Cmd {
	if m.startupOpen {
		return tea.Batch(loadModalCmd(m.root), tickCmd())
	}
	return tea.Batch(loadTaskCmd(m.root, m.taskName), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Re-clamp the log modal's scroll offset against the new height.
		// Without this, shrinking the terminal while the modal is open
		// leaves logScroll pointing past maxLogScroll(), causing the body
		// to render blank until the user presses j/k/G.
		if m.mode == modeLogModal && m.logScroll > m.maxLogScroll() {
			m.logScroll = m.maxLogScroll()
		}
		return m, nil

	case loadedMsg:
		m.loaded = true
		if msg.err != nil && msg.task.Path == "" {
			m.loadErr = msg.err.Error()
		} else {
			m.loadErr = ""
			m.task = msg.task
		}
		return m, nil

	case modalListMsg:
		m.modalItems = msg.items
		m.modalCursor = 0
		m.mode = modeModal
		if m.startupOpen {
			m.startupOpen = false
		}
		return m, nil

	case tickMsg:
		// Only reload task data when on the main view with an active task.
		// Modal-open (task switch or log) and no-task states only re-arm
		// the ticker so the user's scroll / selection is never disturbed.
		if m.mode == modeMain && m.taskName != "" {
			return m, tea.Batch(loadTaskCmd(m.root, m.taskName), tickCmd())
		}
		return m, tickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeModal:
		switch msg.String() {
		case "esc", "q":
			if m.taskName == "" {
				// nothing to fall back to
				return m, tea.Quit
			}
			m.mode = modeMain
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.modalCursor < len(m.modalItems)-1 {
				m.modalCursor++
			}
			return m, nil
		case "k", "up":
			if m.modalCursor > 0 {
				m.modalCursor--
			}
			return m, nil
		case "enter":
			if len(m.modalItems) == 0 {
				return m, nil
			}
			sel := m.modalItems[m.modalCursor]
			m.taskName = sel.Name
			m.mode = modeMain
			m.loaded = false
			m.logScroll = 0
			return m, loadTaskCmd(m.root, m.taskName)
		}
		return m, nil

	case modeLogModal:
		switch msg.String() {
		case "esc", "q":
			m.mode = modeMain
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			max := m.maxLogScroll()
			if m.logScroll < max {
				m.logScroll++
			}
			return m, nil
		case "k", "up":
			if m.logScroll > 0 {
				m.logScroll--
			}
			return m, nil
		case "g", "home":
			m.logScroll = 0
			return m, nil
		case "G", "end":
			m.logScroll = m.maxLogScroll()
			return m, nil
		}
		return m, nil

	default: // modeMain
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if m.taskName == "" {
				return m, loadModalCmd(m.root)
			}
			return m, loadTaskCmd(m.root, m.taskName)
		case "t":
			return m, loadModalCmd(m.root)
		case "l", "L":
			if m.taskName == "" {
				return m, nil
			}
			// open log modal pinned to the latest line.
			m.mode = modeLogModal
			m.logScroll = m.maxLogScroll()
			return m, nil
		}
	}
	return m, nil
}

// maxLogScroll returns the largest valid logScroll offset given the
// current terminal height. Computed against logModalBodyHeight so the
// last line is always reachable but never goes past it.
func (m Model) maxLogScroll() int {
	n := len(m.task.LogTail)
	body := m.logModalBodyHeight()
	if n <= body {
		return 0
	}
	return n - body
}
