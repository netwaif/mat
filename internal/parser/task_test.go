package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netwaif/mat/internal/model"
)

func TestBuildWorkerResultArtifactDetection(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantState  model.WorkerState
		wantResult string
	}{
		{
			name: "result md wins",
			files: map[string]string{
				"brief.md":           "brief",
				"result.md":          "summary",
				"result.tokens.json": "[]",
			},
			wantState:  model.StateDone,
			wantResult: "result.md",
		},
		{
			name: "tokens artifact is done",
			files: map[string]string{
				"brief.md":           "brief",
				"result.tokens.json": "[]",
			},
			wantState:  model.StateDone,
			wantResult: "result.tokens.json",
		},
		{
			name: "output artifact is done",
			files: map[string]string{
				"brief.md":           "brief",
				"result.output.json": "{}",
			},
			wantState:  model.StateDone,
			wantResult: "result.output.json",
		},
		{
			name: "generic artifact is done",
			files: map[string]string{
				"brief.md":      "brief",
				"result.foo.md": "done",
			},
			wantState:  model.StateDone,
			wantResult: "result.foo.md",
		},
		{
			name: "partial only is running",
			files: map[string]string{
				"brief.md":                  "brief",
				"result.partial-05-11.json": "{}",
			},
			wantState: model.StateRunning,
		},
		{
			name: "brief only is running",
			files: map[string]string{
				"brief.md": "brief",
			},
			wantState: model.StateRunning,
		},
		{
			name: "empty result md does not complete",
			files: map[string]string{
				"brief.md":  "brief",
				"result.md": "",
			},
			wantState: model.StateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			workersDir := filepath.Join(root, "workers")
			wdir := filepath.Join(workersDir, "gemini")
			if err := os.MkdirAll(wdir, 0o755); err != nil {
				t.Fatal(err)
			}
			logPath := filepath.Join(root, "log.md")
			if err := os.WriteFile(logPath, []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
			for name, body := range tt.files {
				if err := os.WriteFile(filepath.Join(wdir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got := buildWorker(workersDir, "gemini", logPath)
			if got.State != tt.wantState {
				t.Fatalf("state = %v, want %v", got.State, tt.wantState)
			}
			if tt.wantResult == "" {
				if got.ResultPath != "" {
					t.Fatalf("result path = %q, want empty", got.ResultPath)
				}
				return
			}
			if filepath.Base(got.ResultPath) != tt.wantResult {
				t.Fatalf("result path = %q, want basename %q", got.ResultPath, tt.wantResult)
			}
			if got.ResultSize == 0 {
				t.Fatalf("result size should be non-zero")
			}
		})
	}
}
