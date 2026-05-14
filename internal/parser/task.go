package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/netwaif/mat/internal/model"
)

// LoadTask reads tasks/<name>/{task.md,log.md,workers/*} and returns a Task
// snapshot. Never panics: parse failures degrade to status="unknown".
func LoadTask(root, name string) (model.Task, error) {
	taskDir := filepath.Join(root, "tasks", name)
	taskMD := filepath.Join(taskDir, "task.md")

	t := model.Task{
		Name: name,
		Path: taskMD,
	}

	st, err := os.Stat(taskMD)
	if err != nil {
		t.Status = "unknown"
		t.ParseError = "task.md not found"
		return t, err
	}
	t.UpdatedAt = st.ModTime()

	header, ok := readYAMLHeader(taskMD)
	if !ok {
		t.Status = "unknown"
		t.ParseError = "task.md YAML header missing or invalid"
	} else {
		t.Status = strings.TrimSpace(header["status"])
		if t.Status == "" {
			t.Status = "unknown"
		}
	}

	t.Goal = readGoal(taskMD)

	logPath := filepath.Join(taskDir, "log.md")
	// log lines (display only — UI slices to fit the available height
	// for the main view, or shows the full list in the log modal).
	t.LogTail = readLogLines(logPath)

	// workers
	planned := readPlannedWorkers(taskMD)
	workersDir := filepath.Join(taskDir, "workers")
	onDisk := map[string]bool{}

	entries, _ := os.ReadDir(workersDir)
	var diskWorkers []model.Worker
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		onDisk[name] = true
		w := buildWorker(workersDir, name, logPath)
		// also fill purpose from planned if brief 첫 줄이 비었으면
		if w.Purpose == "" {
			for _, p := range planned {
				if p.Role == name {
					w.Purpose = p.Purpose
					break
				}
			}
		}
		diskWorkers = append(diskWorkers, w)
	}

	// planned-only (no dir): show as pending
	var plannedOnly []model.Worker
	for _, p := range planned {
		if onDisk[p.Role] {
			continue
		}
		plannedOnly = append(plannedOnly, model.Worker{
			Role:        p.Role,
			State:       model.StatePending,
			Purpose:     p.Purpose,
			FromPlanned: true,
		})
	}

	// stable order: planned order first, then disk-only sorted by name.
	order := map[string]int{}
	for i, p := range planned {
		order[p.Role] = i
	}
	all := append(diskWorkers, plannedOnly...)
	sort.SliceStable(all, func(i, j int) bool {
		oi, iok := order[all[i].Role]
		oj, jok := order[all[j].Role]
		switch {
		case iok && jok:
			return oi < oj
		case iok:
			return true
		case jok:
			return false
		default:
			return all[i].Role < all[j].Role
		}
	})
	t.Workers = all

	return t, nil
}

func readGoal(taskMD string) string {
	f, err := os.Open(taskMD)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inGoal := false
	var lines []string
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			if inGoal {
				break
			}
			if strings.TrimSpace(strings.TrimPrefix(trim, "## ")) == "Goal" {
				inGoal = true
				continue
			}
		}
		if inGoal {
			if trim == "" && len(lines) == 0 {
				continue
			}
			lines = append(lines, line)
		}
	}
	// trim trailing blank lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// readLogLines returns every non-blank, non-comment, non-heading line from
// log.md in chronological order. The UI is responsible for slicing (tail
// for the main view, full list for the log modal).
func readLogLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var kept []string
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "<!--") {
			continue
		}
		if strings.HasPrefix(trim, "#") {
			continue
		}
		kept = append(kept, trim)
	}
	return kept
}

func buildWorker(workersDir, role, logPath string) model.Worker {
	w := model.Worker{Role: role}
	wdir := filepath.Join(workersDir, role)

	briefPath := filepath.Join(wdir, "brief.md")
	resultPath := filepath.Join(wdir, "result.md")

	if st, err := os.Stat(briefPath); err == nil && !st.IsDir() {
		w.HasBrief = true
		w.BriefPath = briefPath
		w.BriefSize = st.Size()
		if data, err := os.ReadFile(briefPath); err == nil {
			w.BriefChars = utf8.RuneCountInString(string(data))
			w.Purpose = firstMeaningfulLine(string(data))
		}
		if st.ModTime().After(w.UpdatedAt) {
			w.UpdatedAt = st.ModTime()
		}
	}
	if st, err := os.Stat(resultPath); err == nil && !st.IsDir() {
		if st.Size() > 0 {
			w.HasResult = true
		}
		w.ResultPath = resultPath
		w.ResultSize = st.Size()
		if st.ModTime().After(w.UpdatedAt) {
			w.UpdatedAt = st.ModTime()
		}
	}

	// state derivation
	switch {
	case workerHasError(role, logPath):
		w.State = model.StateError
	case w.HasResult:
		w.State = model.StateDone
	case w.HasBrief:
		w.State = model.StateRunning
	default:
		w.State = model.StatePending
	}
	return w
}

// workerHasError scans the entire log.md (same filters as readLogLines —
// blank / "#" / "<!--" lines excluded) in reverse. The LATEST line that
// mentions the role wins; if it carries [ERROR], the worker is errored.
// Missing or unreadable log file → false (no error state).
func workerHasError(role, logPath string) bool {
	f, err := os.Open(logPath)
	if err != nil {
		return false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var kept []string
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "<!--") {
			continue
		}
		if strings.HasPrefix(trim, "#") {
			continue
		}
		kept = append(kept, trim)
	}
	if err := sc.Err(); err != nil {
		return false
	}
	for i := len(kept) - 1; i >= 0; i-- {
		ln := kept[i]
		if !strings.Contains(ln, role) {
			continue
		}
		return strings.Contains(ln, "[ERROR]")
	}
	return false
}

func firstMeaningfulLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "<!--") {
			continue
		}
		return line
	}
	return ""
}

// PickActiveTask implements DESIGN.md activation order:
//  1. (caller already handled arg)
//  2. task.md status==in_progress|waiting_* — newest task.md mtime
//  3. "" — caller opens modal
func PickActiveTask(root string) string {
	tasksDir := filepath.Join(root, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return ""
	}
	type cand struct {
		name string
		mt   time.Time
	}
	var cands []cand
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		md := filepath.Join(tasksDir, name, "task.md")
		st, err := os.Stat(md)
		if err != nil {
			continue
		}
		hdr, ok := readYAMLHeader(md)
		if !ok {
			continue
		}
		status := strings.TrimSpace(hdr["status"])
		if status == "in_progress" || strings.HasPrefix(status, "waiting_") {
			cands = append(cands, cand{name, st.ModTime()})
		}
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mt.After(cands[j].mt) })
	return cands[0].name
}

// ListTasks returns every task dir under root/tasks/ with status + mtime,
// sorted by mtime desc. Used by the task-switch modal.
func ListTasks(root string) []model.TaskBrief {
	tasksDir := filepath.Join(root, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil
	}
	var out []model.TaskBrief
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		md := filepath.Join(tasksDir, name, "task.md")
		st, err := os.Stat(md)
		if err != nil {
			continue
		}
		hdr, _ := readYAMLHeader(md)
		status := "unknown"
		if hdr != nil {
			if s := strings.TrimSpace(hdr["status"]); s != "" {
				status = s
			}
		}
		out = append(out, model.TaskBrief{
			Name:      name,
			Status:    status,
			UpdatedAt: st.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
