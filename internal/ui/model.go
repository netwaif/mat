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
		// Modal-open or no-task states only re-arm the ticker.
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
			return m, loadTaskCmd(m.root, m.taskName)
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
		}
	}
	return m, nil
}
