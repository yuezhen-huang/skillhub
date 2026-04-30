package storage

import (
	"context"

	"github.com/yuezhen-huang/skillhub/internal/models"
)

// Store defines the interface for skill persistence
type Store interface {
	// SaveSkill saves a skill to storage
	SaveSkill(ctx context.Context, skill *models.Skill) error

	// GetSkill retrieves a skill by ID
	GetSkill(ctx context.Context, id string) (*models.Skill, error)

	// GetSkillByName retrieves a skill by name
	GetSkillByName(ctx context.Context, name string) (*models.Skill, error)

	// ListSkills lists all skills
	ListSkills(ctx context.Context) ([]*models.Skill, error)

	// DeleteSkill deletes a skill by ID
	DeleteSkill(ctx context.Context, id string) error

	// UpdateSkillStatus updates a skill's status
	UpdateSkillStatus(ctx context.Context, id string, status models.SkillStatus) error

	// UpdateSkillProcess updates a skill's process info
	UpdateSkillProcess(ctx context.Context, id string, process *models.ProcessInfo) error

	// UpdateSkillRepository updates a skill's repository info
	UpdateSkillRepository(ctx context.Context, id string, repo *models.Repository) error

	// Close closes the storage connection
	Close() error
}
