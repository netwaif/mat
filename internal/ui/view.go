package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/netwaif/mat/internal/model"
)

var (
	colorBorder  = lipgloss.Color("240")
	colorAccent  = lipgloss.Color("39")
	colorMuted   = lipgloss.Color("245")
	colorWarn    = lipgloss.Color("214")
	colorErr     = lipgloss.Color("203")
	colorOK      = lipgloss.Color("42")

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMuted)

	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	statusStyle = func(s string) lipgloss.Style {
		base := lipgloss.NewStyle().Bold(true)
		switch {
		case s == "done":
			return base.Foreground(colorOK)
		case strings.HasPrefix(s, "waiting_"):
			return base.Foreground(colorWarn)
		case s == "in_progress":
			return base.Foreground(colorAccent)
		case s == "unknown":
			return base.Foreground(colorErr)
		default:
			return base.Foreground(colorMuted)
		}
	}
)

func (m Model) View() string {
	if !m.loaded && m.mode != modeModal {
		return mutedStyle.Render("loading…  (root: " + m.root + ")")
	}
	mainView := m.renderMain()
	if m.mode == modeModal {
		return overlayCenter(mainView, m.renderModal(), m.width, m.height)
	}
	return mainView
}

func (m Model) renderMain() string {
	width := m.width
	if width <= 0 {
		width = 78
	}
	inner := width - 4
	if inner < 30 {
		inner = 30
	}

	var b strings.Builder

	// header
	title := headerStyle.Render("mat") + "  —  Task: " + boldStr(m.taskName)
	statusLine := ""
	if m.taskName != "" {
		statusLine = "Status: " + statusStyle(m.task.Status).Render(m.task.Status) +
			"   ·   Updated: " + m.task.UpdatedAt.Format("15:04")
	} else if m.loadErr != "" {
		statusLine = mutedStyle.Render(m.loadErr)
	}
	header := title + "\n" + statusLine
	b.WriteString(boxStyle.Width(inner).Render(header))
	b.WriteString("\n")

	if m.taskName == "" {
		b.WriteString(mutedStyle.Render("No active task. Press [t] to choose, [q] to quit."))
		b.WriteString("\n")
		b.WriteString(footerHelp())
		return b.String()
	}

	if m.task.ParseError != "" {
		warn := lipgloss.NewStyle().Foreground(colorErr).Render("⚠ " + m.task.ParseError)
		b.WriteString(boxStyle.Width(inner).Render(warn))
		b.WriteString("\n")
	}

	// goal
	goal := m.task.Goal
	if goal == "" {
		goal = mutedStyle.Render("(no Goal section)")
	}
	b.WriteString(boxStyle.Width(inner).Render(sectionTitleStyle.Render("Goal") + "\n" + goal))
	b.WriteString("\n")

	// workers
	b.WriteString(boxStyle.Width(inner).Render(renderWorkers(m.task.Workers)))
	b.WriteString("\n")

	// log tail
	b.WriteString(boxStyle.Width(inner).Render(renderLog(m.task.LogTail)))
	b.WriteString("\n")

	b.WriteString(footerHelp())
	return b.String()
}

func renderWorkers(ws []model.Worker) string {
	var b strings.Builder
	b.WriteString(sectionTitleStyle.Render("Workers"))
	b.WriteString("\n")
	if len(ws) == 0 {
		b.WriteString(mutedStyle.Render("  (none)"))
		return b.String()
	}
	for i, w := range ws {
		if i > 0 {
			b.WriteString("\n")
		}
		head := fmt.Sprintf("  %s %-18s  %s",
			w.State.Icon(),
			w.Role,
			w.State.Label(),
		)
		if !w.UpdatedAt.IsZero() {
			head += "   " + mutedStyle.Render(w.UpdatedAt.Format("15:04"))
		}
		b.WriteString(head)
		if w.Purpose != "" {
			b.WriteString("\n")
			b.WriteString("        " + truncate(w.Purpose, 60))
		}
		switch {
		case w.HasResult:
			b.WriteString("\n")
			b.WriteString("        " + mutedStyle.Render(
				fmt.Sprintf("result.md: %s (%s)", shortPath(w.ResultPath), humanSize(w.ResultSize))))
		case w.HasBrief:
			b.WriteString("\n")
			b.WriteString("        " + mutedStyle.Render(
				fmt.Sprintf("brief.md: %s (%d자)", shortPath(w.BriefPath), w.BriefChars)))
		case w.FromPlanned:
			b.WriteString("\n")
			b.WriteString("        " + mutedStyle.Render("실행 예정"))
		}
	}
	return b.String()
}

func renderLog(tail []string) string {
	var b strings.Builder
	b.WriteString(sectionTitleStyle.Render("Recent log (last 5)"))
	b.WriteString("\n")
	if len(tail) == 0 {
		b.WriteString(mutedStyle.Render("  (empty)"))
		return b.String()
	}
	for _, ln := range tail {
		b.WriteString("  ")
		b.WriteString(truncate(ln, 80))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func footerHelp() string {
	return mutedStyle.Render(" 자동 갱신 2s · [r] 즉시   [t] 작업 전환   [q] 종료")
}

func (m Model) renderModal() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Tasks") + "  " + mutedStyle.Render("(j/k 이동, enter 선택, esc 취소)"))
	b.WriteString("\n\n")
	if len(m.modalItems) == 0 {
		b.WriteString(mutedStyle.Render("  (no tasks found under " + m.root + "/tasks)"))
	}
	for i, it := range m.modalItems {
		cursor := "  "
		row := fmt.Sprintf("%-28s  %s", truncate(it.Name, 28), it.Status)
		if i == m.modalCursor {
			cursor = "→ "
			row = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(row)
		}
		b.WriteString(cursor)
		b.WriteString(row)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(" [esc] 취소"))
	return modalStyle.Render(b.String())
}

// --- helpers ---

func boldStr(s string) string {
	return lipgloss.NewStyle().Bold(true).Render(s)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func shortPath(p string) string {
	// surface only "workers/<role>/file"
	parts := strings.Split(p, "/")
	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == "workers" {
			return strings.Join(parts[i:], "/")
		}
	}
	return p
}

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// overlayCenter draws `over` centered on top of `base` by replacing the
// middle rows. bubbletea has no native overlay primitive; we use a coarse
// row-replace which is fine for the modal-on-static-view MVP case.
func overlayCenter(base, over string, w, h int) string {
	if w <= 0 || h <= 0 {
		// no size yet — fall back to concatenation
		return over + "\n\n" + base
	}
	baseLines := strings.Split(base, "\n")
	overLines := strings.Split(over, "\n")
	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	overH := len(overLines)
	if overH > h {
		overH = h
		overLines = overLines[:h]
	}
	overW := 0
	for _, ln := range overLines {
		if l := lipgloss.Width(ln); l > overW {
			overW = l
		}
	}
	startRow := (h - overH) / 2
	if startRow < 0 {
		startRow = 0
	}
	startCol := (w - overW) / 2
	if startCol < 0 {
		startCol = 0
	}

	for i, ln := range overLines {
		row := startRow + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = padOrCutLeft(ln, startCol)
	}
	return strings.Join(baseLines, "\n")
}

// padOrCutLeft prepends `pad` spaces to s.
func padOrCutLeft(s string, pad int) string {
	if pad <= 0 {
		return s
	}
	return strings.Repeat(" ", pad) + s
}
