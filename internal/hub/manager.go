package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/skillhub/skill-hub/internal/gitlab"
	"github.com/skillhub/skill-hub/internal/models"
	"github.com/skillhub/skill-hub/internal/storage"
	sk "github.com/skillhub/skill-hub/internal/skill"
	"github.com/skillhub/skill-hub/pkg/config"

	"github.com/google/uuid"
)

// Manager handles skill lifecycle operations
type Manager struct {
	store     storage.Store
	repoMgr   *gitlab.RepositoryManager
	runtime   *sk.Runtime
	cfg       *config.Config
}

// NewManager creates a new skill manager
func NewManager(store storage.Store, repoMgr *gitlab.RepositoryManager, runtime *sk.Runtime, cfg *config.Config) *Manager {
	return &Manager{
		store:     store,
		repoMgr:   repoMgr,
		runtime:   runtime,
		cfg:       cfg,
	}
}

// Add adds a new skill from GitLab
func (m *Manager) Add(ctx context.Context, spec *models.SkillSpec) (*models.Skill, error) {
	if existing, _ := m.store.GetSkillByName(ctx, spec.Name); existing != nil {
		return nil, fmt.Errorf("skill with name %q already exists", spec.Name)
	}

	id := uuid.New().String()
	skillPath := filepath.Join(m.cfg.Skill.SkillsDir, id)

	repo, err := m.repoMgr.Clone(ctx, spec.GitLabURL, skillPath)
	if err != nil {
		return nil, fmt.Errorf("clone failed: %w", err)
	}

	repo.ID = id

	if spec.VersionRef != "" {
		if err := m.switchRef(ctx, repo, spec.VersionRef); err != nil {
			return nil, fmt.Errorf("version switch failed: %w", err)
		}
	}

	skill := &models.Skill{
		ID:         id,
		Name:       spec.Name,
		Version:    repo.Commit[:8],
		Repository: repo,
		Status:     models.SkillStatusStopped,
		Config:     spec.Config,
	}

	if err := m.store.SaveSkill(ctx, skill); err != nil {
		return nil, err
	}

	return skill, nil
}

// Remove removes a skill
func (m *Manager) Remove(ctx context.Context, id string) error {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return err
	}

	if skill.Status == models.SkillStatusRunning {
		if err := m.Stop(ctx, id); err != nil {
			return err
		}
	}

	if err := m.store.DeleteSkill(ctx, id); err != nil {
		return err
	}

	if skill.Repository != nil && skill.Repository.Path != "" {
		// Note: We don't automatically delete the directory for safety
	}

	return nil
}

// Get gets a skill by ID
func (m *Manager) Get(ctx context.Context, id string) (*models.Skill, error) {
	return m.store.GetSkill(ctx, id)
}

// List lists all skills
func (m *Manager) List(ctx context.Context) ([]*models.Skill, error) {
	return m.store.ListSkills(ctx)
}

// Start starts a skill
func (m *Manager) Start(ctx context.Context, id string) error {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return err
	}

	if skill.Status == models.SkillStatusRunning {
		return nil
	}

	if err := m.store.UpdateSkillStatus(ctx, id, models.SkillStatusStarting); err != nil {
		return err
	}

	process, err := m.runtime.Spawn(ctx, skill)
	if err != nil {
		m.store.UpdateSkillStatus(ctx, id, models.SkillStatusError)
		return err
	}

	skill.Process = process
	if err := m.store.UpdateSkillProcess(ctx, id, process); err != nil {
		m.runtime.Kill(ctx, skill)
		return err
	}

	if err := m.store.UpdateSkillStatus(ctx, id, models.SkillStatusRunning); err != nil {
		m.runtime.Kill(ctx, skill)
		return err
	}

	return nil
}

// Stop stops a skill
func (m *Manager) Stop(ctx context.Context, id string) error {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return err
	}

	if skill.Status == models.SkillStatusStopped {
		return nil
	}

	if err := m.store.UpdateSkillStatus(ctx, id, models.SkillStatusStopping); err != nil {
		return err
	}

	if err := m.runtime.Kill(ctx, skill); err != nil {
		m.store.UpdateSkillStatus(ctx, id, models.SkillStatusError)
		return err
	}

	if err := m.store.UpdateSkillStatus(ctx, id, models.SkillStatusStopped); err != nil {
		return err
	}

	if err := m.store.UpdateSkillProcess(ctx, id, nil); err != nil {
		return err
	}

	return nil
}

// Restart restarts a skill
func (m *Manager) Restart(ctx context.Context, id string) error {
	if err := m.Stop(ctx, id); err != nil {
		return err
	}
	return m.Start(ctx, id)
}

// Status gets a skill's status
func (m *Manager) Status(ctx context.Context, id string) (models.SkillStatus, error) {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return models.SkillStatusUnknown, err
	}
	return skill.Status, nil
}

// SwitchVersion switches to a specific version (branch, tag, or commit)
func (m *Manager) SwitchVersion(ctx context.Context, id, ref string) error {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return err
	}

	if skill.Status == models.SkillStatusRunning {
		if err := m.Stop(ctx, id); err != nil {
			return err
		}
	}

	if err := m.repoMgr.Fetch(ctx, skill.Repository); err != nil {
		return err
	}

	if err := m.switchRef(ctx, skill.Repository, ref); err != nil {
		return err
	}

	skill.Version = ref
	if len(skill.Repository.Commit) >= 8 {
		skill.Version = skill.Repository.Commit[:8]
	}

	if err := m.store.UpdateSkillRepository(ctx, id, skill.Repository); err != nil {
		return err
	}

	return nil
}

// ListVersions lists available versions (tags and branches)
func (m *Manager) ListVersions(ctx context.Context, id string) ([]string, []string, error) {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	if err := m.repoMgr.Fetch(ctx, skill.Repository); err != nil {
		return nil, nil, err
	}

	tags, err := m.repoMgr.ListTags(ctx, skill.Repository)
	if err != nil {
		return nil, nil, err
	}

	branches, err := m.repoMgr.ListBranches(ctx, skill.Repository)
	if err != nil {
		return nil, nil, err
	}

	return tags, branches, nil
}

// PullLatest pulls the latest changes for the current branch
func (m *Manager) PullLatest(ctx context.Context, id string) (string, error) {
	skill, err := m.store.GetSkill(ctx, id)
	if err != nil {
		return "", err
	}

	if skill.Status == models.SkillStatusRunning {
		if err := m.Stop(ctx, id); err != nil {
			return "", err
		}
	}

	if err := m.repoMgr.Pull(ctx, skill.Repository); err != nil {
		return "", err
	}

	if len(skill.Repository.Commit) >= 8 {
		skill.Version = skill.Repository.Commit[:8]
	}

	if err := m.store.UpdateSkillRepository(ctx, id, skill.Repository); err != nil {
		return "", err
	}

	return skill.Repository.Commit, nil
}

func (m *Manager) switchRef(ctx context.Context, repo *models.Repository, ref string) error {
	// Try as tag first
	if err := m.repoMgr.CheckoutTag(ctx, repo, ref); err == nil {
		return nil
	}

	// Try as branch
	if err := m.repoMgr.CheckoutBranch(ctx, repo, ref); err == nil {
		return nil
	}

	// Try as commit
	if err := m.repoMgr.CheckoutCommit(ctx, repo, ref); err != nil {
		return fmt.Errorf("ref %q not found as tag, branch, or commit", ref)
	}

	return nil
}

// HealthCheckAll checks health of all running skills
func (m *Manager) HealthCheckAll(ctx context.Context) map[string]bool {
	skills, err := m.List(ctx)
	if err != nil {
		return nil
	}

	results := make(map[string]bool)
	for _, skill := range skills {
		if skill.Status != models.SkillStatusRunning {
			continue
		}
		healthy, _ := m.runtime.HealthCheck(ctx, skill)
		results[skill.ID] = healthy
	}

	return results
}
