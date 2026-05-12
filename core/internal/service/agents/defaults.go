package agents

import "strings"

type DefaultTemplate struct {
	AgentType    string
	Role         string
	DefaultModel string
	Scopes       []string
	Skills       []string
	Models       []string
}

var defaultTemplates = map[string]DefaultTemplate{
	"claude-code": {
		AgentType:    "claude-code",
		Role:         "coordinator",
		DefaultModel: "claude-sonnet",
		Scopes: []string{
			"task:read:tree",
			"task:read:delegated",
			"task:create",
			"task:delegate:codex",
			"task:delegate:gemini",
			"task:review",
			"room:read",
			"room:write",
			"room:mention",
			"room:pin",
			"room:summarize",
			"memory:read",
			"memory:search",
			"memory:write:decision",
			"memory:write:summary",
		},
		Skills: []string{"project-coordinator", "review", "code-review"},
		Models: []string{"claude-sonnet", "claude-opus"},
	},
	"codex": {
		AgentType:    "codex",
		Role:         "worker",
		DefaultModel: "gpt-5-codex",
		Scopes: []string{
			"task:read:own",
			"task:start:delegated",
			"task:update:delegated",
			"task:submit:delegated",
			"room:read",
			"room:write",
			"room:mention",
			"memory:read",
			"memory:search",
			"memory:write:summary",
		},
		Skills: []string{"backend-implementation"},
		Models: []string{"gpt-5-codex"},
	},
	"gemini": {
		AgentType:    "gemini",
		Role:         "worker",
		DefaultModel: "gemini-2.5-pro",
		Scopes: []string{
			"task:read:own",
			"task:start:delegated",
			"task:update:delegated",
			"task:submit:delegated",
			"room:read",
			"room:write",
			"room:history",
			"room:summarize",
			"memory:read",
			"memory:search",
			"memory:write:summary",
		},
		Skills: []string{"document-generation"},
		Models: []string{"gemini-2.5-pro"},
	},
}

func DefaultTemplateByType(agentType string) (DefaultTemplate, bool) {
	template, ok := defaultTemplates[strings.TrimSpace(agentType)]
	if !ok {
		return DefaultTemplate{}, false
	}
	template.Scopes = uniqueStrings(template.Scopes)
	template.Skills = uniqueStrings(template.Skills)
	template.Models = uniqueStrings(template.Models)
	return template, true
}
