package models

import "time"

// SkillKind describes what a skill is and how it can run.
type SkillKind string

const (
	// SkillKindGo indicates a runnable Go skill (git/go project, can be built and started).
	SkillKindGo SkillKind = "go"
	// SkillKindDoc indicates a documentation-only skill (e.g. a directory with SKILL.md).
	SkillKindDoc SkillKind = "doc"
)

// SkillStatus represents the status of a skill
type SkillStatus string

const (
	SkillStatusUnknown  SkillStatus = "unknown"
	SkillStatusStopped  SkillStatus = "stopped"
	SkillStatusStarting SkillStatus = "starting"
	SkillStatusRunning  SkillStatus = "running"
	SkillStatusStopping SkillStatus = "stopping"
	SkillStatusError    SkillStatus = "error"
)

// Skill represents a managed skill
type Skill struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Kind        SkillKind          `json:"kind"`
	SourcePath  string             `json:"source_path"`
	Repository  *Repository       `json:"repository"`
	Status      SkillStatus       `json:"status"`
	Process     *ProcessInfo      `json:"process,omitempty"`
	Config      map[string]string `json:"config"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Repository represents Git repository info
type Repository struct {
	ID       string     `json:"id"`
	URL      string     `json:"url"`
	Remote   string     `json:"remote"`
	Path     string     `json:"path"`
	Branch   string     `json:"branch"`
	Tag      string     `json:"tag,omitempty"`
	Commit   string     `json:"commit,omitempty"`
	LastPull *time.Time `json:"last_pull,omitempty"`
}

// ProcessInfo holds running process information
type ProcessInfo struct {
	PID        int       `json:"pid"`
	Port       int       `json:"port"`
	RPCAddress string    `json:"rpc_address"`
	StartedAt  time.Time `json:"started_at"`
}

// SkillSpec is used to add a new skill
type SkillSpec struct {
	Name       string            `json:"name"`
	GitLabURL  string            `json:"gitlab_url"`
	VersionRef string            `json:"version_ref"`
	Config     map[string]string `json:"config"`
}
