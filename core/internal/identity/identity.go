package identity

import "strings"

type SenderType string

const (
	SenderTypeOperator SenderType = "operator"
	SenderTypeAgent    SenderType = "agent"
	SenderTypeSystem   SenderType = "system"
)

type AgentIdentity struct {
	TokenID         string
	AgentID         string
	AgentType       string
	Role            string
	Scopes          []string
	AllowedProjects []string
}

func (a AgentIdentity) HasScope(scope string) bool {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return false
	}
	for _, item := range a.Scopes {
		if strings.TrimSpace(item) == scope {
			return true
		}
	}
	return false
}

func (a AgentIdentity) CanAccessProject(projectID string) bool {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false
	}
	for _, item := range a.AllowedProjects {
		if strings.TrimSpace(item) == projectID {
			return true
		}
	}
	return false
}

type Identity struct {
	SenderType    SenderType
	OperatorLabel string
	Agent         AgentIdentity
}

func Operator(label string) Identity {
	return Identity{SenderType: SenderTypeOperator, OperatorLabel: strings.TrimSpace(label)}
}

func System() Identity {
	return Identity{SenderType: SenderTypeSystem}
}

func (i Identity) IsAgent() bool {
	return i.SenderType == SenderTypeAgent
}

func (i Identity) IsOperator() bool {
	return i.SenderType == SenderTypeOperator
}

func (i Identity) IsSystem() bool {
	return i.SenderType == SenderTypeSystem
}
