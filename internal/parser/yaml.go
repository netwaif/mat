package parser

import (
	"bufio"
	"os"
	"strings"
)

// readYAMLHeader extracts flat key:value pairs from the FIRST ```yaml fenced
// block in a markdown file. Nested structures, lists, anchors are ignored
// (only top-level scalar values are returned). Comments (#...) are stripped.
// Returns (map, true) on success. Returns (nil, false) when:
//   - file unreadable
//   - no ```yaml block found
//   - block contains no parseable scalar pairs
//
// This is intentionally tiny — task.md/MVP only needs `status`, and we never
// want to panic.
func readYAMLHeader(path string) (map[string]string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inBlock := false
	out := map[string]string{}

	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)

		if !inBlock {
			if trim == "```yaml" || trim == "``` yaml" {
				inBlock = true
			}
			continue
		}
		// inside the block
		if strings.HasPrefix(trim, "```") {
			break
		}
		// strip comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			// preserve "#" only when inside quotes — MVP: not needed for our keys
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// only accept simple `key: value` at indent 0
		// skip list items / nested mappings
		if strings.HasPrefix(line, "- ") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		// drop empty values (block scalars / nested mappings start that way)
		if val == "" {
			continue
		}
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// readPlannedWorkers extracts `- role: <name>` entries appearing under a
// `planned_workers:` key inside ANY ```yaml fenced block in the document.
// We scan every yaml block in order; the FIRST block that contains a
// `planned_workers:` mapping wins, and parsing stops at the end of that
// block. Order preserved, duplicates removed. Returns nil on any failure
// (never panics).
func readPlannedWorkers(path string) []plannedEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	const (
		stateBefore = iota
		stateInBlock
		stateInPlanned
	)
	state := stateBefore

	var entries []plannedEntry
	var current *plannedEntry
	seen := map[string]bool{}
	matched := false // true once we've entered planned_workers in some block

	flush := func() {
		if current == nil {
			return
		}
		if current.Role != "" && !seen[current.Role] {
			seen[current.Role] = true
			entries = append(entries, *current)
		}
		current = nil
	}

	for sc.Scan() {
		raw := sc.Text()
		trim := strings.TrimSpace(raw)

		switch state {
		case stateBefore:
			if trim == "```yaml" || trim == "``` yaml" {
				state = stateInBlock
			}
		case stateInBlock, stateInPlanned:
			if strings.HasPrefix(trim, "```") {
				// end of this yaml block
				flush()
				if matched {
					// we already consumed the planned_workers block; done.
					return entries
				}
				// otherwise: keep scanning for the next ```yaml block
				state = stateBefore
				continue
			}
			// strip trailing comments
			work := raw
			if idx := strings.Index(work, "#"); idx >= 0 {
				work = work[:idx]
			}
			lineTrim := strings.TrimSpace(work)
			if lineTrim == "" {
				continue
			}

			indent := leadingSpaces(work)

			// entering / leaving planned_workers section
			if indent == 0 && strings.HasPrefix(lineTrim, "planned_workers:") {
				flush()
				state = stateInPlanned
				matched = true
				continue
			}
			if indent == 0 && strings.Contains(lineTrim, ":") {
				// new top-level key — leave planned_workers
				flush()
				state = stateInBlock
				continue
			}

			if state != stateInPlanned {
				continue
			}

			if strings.HasPrefix(lineTrim, "- ") {
				flush()
				current = &plannedEntry{}
				rest := strings.TrimSpace(strings.TrimPrefix(lineTrim, "- "))
				if k, v, ok := splitKV(rest); ok {
					assignPlanned(current, k, v)
				}
				continue
			}
			if current != nil {
				if k, v, ok := splitKV(lineTrim); ok {
					assignPlanned(current, k, v)
				}
			}
		}
	}
	flush()
	return entries
}

type plannedEntry struct {
	Role    string
	Purpose string
}

func splitKV(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return "", "", false
	}
	k := strings.TrimSpace(s[:idx])
	v := strings.TrimSpace(s[idx+1:])
	v = strings.Trim(v, `"'`)
	return k, v, true
}

func assignPlanned(p *plannedEntry, k, v string) {
	switch k {
	case "role":
		p.Role = v
	case "purpose":
		p.Purpose = v
	}
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}
