package agents

import "testing"

func TestDefaultTemplateByType(t *testing.T) {
	cases := []string{"claude-code", "codex", "gemini"}
	for _, item := range cases {
		template, ok := DefaultTemplateByType(item)
		if !ok {
			t.Fatalf("template for %s not found", item)
		}
		if template.AgentType != item {
			t.Fatalf("template agent type mismatch: %s", item)
		}
		if len(template.Scopes) == 0 {
			t.Fatalf("template scopes should not be empty: %s", item)
		}
	}
	if _, ok := DefaultTemplateByType("system"); ok {
		t.Fatalf("system should not expose default template")
	}
}
