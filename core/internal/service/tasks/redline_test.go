package tasks

import "testing"

func TestNormalizeCreateInputDropsDelegateType(t *testing.T) {
	in := CreateTaskInput{
		Title:               "x",
		Description:         "y",
		DelegateToAgentID:   "agt_1",
		DelegateToAgentType: "claude-code",
	}
	got := normalizeCreateInput(in)
	if got.DelegateToAgentType != "" {
		t.Fatalf("delegate type should be ignored, got %q", got.DelegateToAgentType)
	}
}
