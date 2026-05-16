package ui

import (
	"strings"
	"testing"

	"github.com/netwaif/mat/internal/model"
)

func TestStatusStyleReviewingPending(t *testing.T) {
	if got := statusStyle("reviewing").GetForeground(); got != colorReview {
		t.Errorf("statusStyle(reviewing) fg = %v, want colorReview %v", got, colorReview)
	}
	if got := statusStyle("pending").GetForeground(); got != colorMuted {
		t.Errorf("statusStyle(pending) fg = %v, want colorMuted %v", got, colorMuted)
	}
	// existing cases unchanged
	if got := statusStyle("done").GetForeground(); got != colorOK {
		t.Errorf("statusStyle(done) fg = %v, want colorOK %v", got, colorOK)
	}
}

func TestRenderArtifacts(t *testing.T) {
	out := renderArtifacts([]model.Artifact{
		{Name: "review-report.md", Size: 10759},
		{Name: "src", IsDir: true, Count: 3},
	})
	for _, want := range []string{"Artifacts", "review-report.md", "src", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderArtifacts output missing %q:\n%s", want, out)
		}
	}
}
