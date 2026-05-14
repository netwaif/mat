package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/netwaif/mat/internal/model"
)

var (
	colorBorder = lipgloss.Color("240")
	colorAccent = lipgloss.Color("39")
	colorMuted  = lipgloss.Color("245")
	colorWarn   = lipgloss.Color("214")
	colorErr    = lipgloss.Color("203")
	colorOK     = lipgloss.Color("42")

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

// minLogLines is the floor for the main-view log box; we never collapse
// below this even on a tiny (e.g. 80x24) terminal — better to clip the
// box than to show 0 log lines.
const minLogLines = 5

// logBoxChrome accounts for the lipgloss roundedBorder (top+bottom = 2)
// plus the section title row inside the box. Padding is 0 vertically.
const logBoxChrome = 3

func (m Model) View() string {
	if !m.loaded && m.mode != modeModal {
		return mutedStyle.Render("loading…  (root: " + m.root + ")")
	}
	mainView := m.renderMain()
	switch m.mode {
	case modeModal:
		return overlayCenter(mainView, m.renderModal(), m.width, m.height)
	case modeLogModal:
		return overlayCenter(mainView, m.renderLogModal(), m.width, m.height)
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

	// --- pieces above the log box ---
	title := headerStyle.Render("mat") + "  —  Task: " + boldStr(m.taskName)
	statusLine := ""
	if m.taskName != "" {
		statusLine = "Status: " + statusStyle(m.task.Status).Render(m.task.Status) +
			"   ·   Updated: " + m.task.UpdatedAt.Format("15:04")
	} else if m.loadErr != "" {
		statusLine = mutedStyle.Render(m.loadErr)
	}
	headerBox := boxStyle.Width(inner).Render(title + "\n" + statusLine)

	if m.taskName == "" {
		var b strings.Builder
		b.WriteString(headerBox)
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("No active task. Press [t] to choose, [q] to quit."))
		b.WriteString("\n")
		b.WriteString(footerHelp())
		return b.String()
	}

	var parseErrBox string
	if m.task.ParseError != "" {
		warn := lipgloss.NewStyle().Foreground(colorErr).Render("⚠ " + m.task.ParseError)
		parseErrBox = boxStyle.Width(inner).Render(warn)
	}

	goalText := m.task.Goal
	if goalText == "" {
		goalText = mutedStyle.Render("(no Goal section)")
	}
	goalBox := boxStyle.Width(inner).Render(sectionTitleStyle.Render("Goal") + "\n" + goalText)

	workersBox := boxStyle.Width(inner).Render(renderWorkers(m.task.Workers))

	footer := footerHelp()

	// --- compute how many log lines fit in remaining vertical space ---
	// Each non-log block contributes Height(box) + 1 to account for the
	// trailing "\n" written between blocks. Footer is the last line and
	// has no trailing "\n", so it contributes only Height(footer). The
	// log box itself also has a "\n" separator after it (between logBox
	// and footer) — that single newline is the trailing "- 1" below.
	usedRows := lipgloss.Height(headerBox) + 1 // box + trailing \n
	if parseErrBox != "" {
		usedRows += lipgloss.Height(parseErrBox) + 1
	}
	usedRows += lipgloss.Height(goalBox) + 1
	usedRows += lipgloss.Height(workersBox) + 1
	usedRows += lipgloss.Height(footer) // no trailing \n after footer

	logLines := minLogLines
	if m.height > 0 {
		remaining := m.height - usedRows - logBoxChrome - 1
		if remaining > logLines {
			logLines = remaining
		}
	}

	logBox := boxStyle.Width(inner).Render(renderLog(m.task.LogTail, logLines))

	// --- assemble ---
	var b strings.Builder
	b.WriteString(headerBox)
	b.WriteString("\n")
	if parseErrBox != "" {
		b.WriteString(parseErrBox)
		b.WriteString("\n")
	}
	b.WriteString(goalBox)
	b.WriteString("\n")
	b.WriteString(workersBox)
	b.WriteString("\n")
	b.WriteString(logBox)
	b.WriteString("\n")
	b.WriteString(footer)
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

// renderLog renders the tail of the log into the main view. `limit` is the
// maximum number of lines to show (computed from terminal height in
// renderMain). The section title reflects how many we are showing vs total
// so the user knows there is more in the [L] modal.
func renderLog(tail []string, limit int) string {
	if limit < 1 {
		limit = 1
	}
	var shown []string
	if len(tail) > limit {
		shown = tail[len(tail)-limit:]
	} else {
		shown = tail
	}

	var b strings.Builder
	titleText := fmt.Sprintf("Recent log (last %d of %d)", len(shown), len(tail))
	if len(tail) == 0 {
		titleText = "Recent log"
	}
	b.WriteString(sectionTitleStyle.Render(titleText))
	b.WriteString("\n")
	if len(tail) == 0 {
		b.WriteString(mutedStyle.Render("  (empty)"))
		return b.String()
	}
	for _, ln := range shown {
		b.WriteString("  ")
		b.WriteString(truncate(ln, 80))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func footerHelp() string {
	return mutedStyle.Render(" 자동 갱신 2s · [r] 즉시   [t] 작업 전환   [L] 로그   [q] 종료")
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

// logModalBodyHeight is the number of log lines the log modal can show
// at the current terminal height (excludes the modal's own chrome and
// the title/footer lines inside the modal box).
//
// chrome breakdown:
//   doubleBorder top+bottom .... 2
//   title line ................. 1
//   blank line after title ..... 1
//   blank line before footer ... 1
//   footer/indicator line ...... 1
//   safety margin .............. 1
//   ---------------------------------
//   total ...................... 7
// This keeps the modal one row shy of m.height so a stray re-render or
// terminal status row never clips the indicator line.
func (m Model) logModalBodyHeight() int {
	const chrome = 7
	body := m.height - chrome
	if body < 5 {
		body = 5
	}
	return body
}

// renderLogModal renders the full log inside a near-fullscreen modal.
// j/k scroll, g/G jump to top/bottom, esc closes.
func (m Model) renderLogModal() string {
	w := m.width
	if w <= 0 {
		w = 78
	}
	inner := w - 4
	if inner < 30 {
		inner = 30
	}

	body := m.logModalBodyHeight()
	total := len(m.task.LogTail)

	var b strings.Builder
	title := headerStyle.Render("Log") + "  " + mutedStyle.Render(
		fmt.Sprintf("(j/k 스크롤, g/G 처음/끝, esc 닫기) · %d줄", total))
	b.WriteString(title)
	b.WriteString("\n\n")

	if total == 0 {
		b.WriteString(mutedStyle.Render("  (empty)"))
		// pad body so the modal stays at a stable size
		for i := 1; i < body; i++ {
			b.WriteString("\n")
		}
	} else {
		start := m.logScroll
		if start < 0 {
			start = 0
		}
		if start > total-1 {
			start = total - 1
		}
		end := start + body
		if end > total {
			end = total
		}
		shown := m.task.LogTail[start:end]
		for i, ln := range shown {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(truncate(ln, inner-2))
		}
		// pad to body height so the footer stays anchored
		for i := len(shown); i < body; i++ {
			b.WriteString("\n")
		}
		// position indicator
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render(
			fmt.Sprintf(" %d–%d / %d", start+1, end, total)))
		return modalStyle.Width(inner).Render(b.String())
	}

	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render(" [esc] 닫기"))
	return modalStyle.Width(inner).Render(b.String())
}

// --- helpers ---

func boldStr(s string) string {
	return lipgloss.NewStyle().Bold(true).Render(s)
}

// truncate clips s so its visual (cell) width does not exceed n. CJK and
// other wide runes count as 2 cells via lipgloss.Width, matching the way
// the terminal renders them — a rune-count based truncate would let a
// Korean-heavy line wrap to a second row inside a fixed-height box and
// break vertical layout (the log modal regression in particular). When
// the string fits, it is returned unchanged; otherwise we accumulate
// runes until adding the next one would exceed n-1 cells, then append
// "…" (which itself is 1 cell wide).
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	if n == 1 {
		// not enough room for content + ellipsis; emit ellipsis alone
		return "…"
	}
	limit := n - 1 // reserve 1 cell for the ellipsis
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > limit {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteRune('…')
	return b.String()
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
