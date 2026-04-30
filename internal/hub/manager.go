package hub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yuezhen-huang/skillhub/internal/analyzer"
	"github.com/yuezhen-huang/skillhub/internal/gitlab"
	"github.com/yuezhen-huang/skillhub/internal/models"
	"github.com/yuezhen-huang/skillhub/internal/scanner"
	sk "github.com/yuezhen-huang/skillhub/internal/skill"
	"github.com/yuezhen-huang/skillhub/internal/storage"
	"github.com/yuezhen-huang/skillhub/pkg/config"

	"github.com/google/uuid"
)

// Manager handles skill lifecycle operations
type Manager struct {
	store    storage.Store
	repoMgr  *gitlab.RepositoryManager
	runtime  *sk.Runtime
	cfg      *config.Config
	scanner  *scanner.Scanner
	analyzer *analyzer.Analyzer
}

// NewManager creates a new skill manager
func NewManager(store storage.Store, repoMgr *gitlab.RepositoryManager, runtime *sk.Runtime, cfg *config.Config) *Manager {
	return &Manager{
		store:    store,
		repoMgr:  repoMgr,
		runtime:  runtime,
		cfg:      cfg,
		scanner:  scanner.NewScanner(cfg),
		analyzer: analyzer.NewAnalyzer(runtime),
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
		Kind:       models.SkillKindGo,
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

	if skill.Kind == models.SkillKindDoc {
		return fmt.Errorf("skill %q is documentation-only and cannot be started", skill.Name)
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

// ScanResult contains the results of a scan operation
type ScanResult struct {
	Discovered    []*scanner.DiscoveredSkill
	ImportedCount int
	SkippedCount  int
}

// ScanSkills scans for existing skills and optionally imports them
func (m *Manager) ScanSkills(ctx context.Context, importAll bool) (*ScanResult, error) {
	// Get existing skills first
	existingSkills, err := m.List(ctx)
	if err != nil {
		return nil, err
	}

	existingMap := make(map[string]bool)
	for _, skill := range existingSkills {
		existingMap[skill.Name] = true
	}

	// Scan the directory
	discovered, err := m.scanner.ScanSkillsDir(ctx, existingMap)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		Discovered: discovered,
	}

	// Import if requested
	if importAll {
		for _, d := range discovered {
			if d.AlreadyImported || !d.IsValidSkill {
				result.SkippedCount++
				continue
			}

			if _, err := m.importFromPath(ctx, d.Path, d.Name); err == nil {
				result.ImportedCount++
			} else {
				result.SkippedCount++
			}
		}
	} else {
		// Just count
		for _, d := range discovered {
			if d.AlreadyImported || !d.IsValidSkill {
				result.SkippedCount++
			}
		}
	}

	return result, nil
}

// importFromPath imports a skill from an existing directory
func (m *Manager) importFromPath(ctx context.Context, path, name string) (*models.Skill, error) {
	id := uuid.New().String()

	skill := &models.Skill{
		ID:         id,
		Name:       name,
		Kind:       models.SkillKindDoc,
		SourcePath: path,
		Status:     models.SkillStatusStopped,
		Config:     make(map[string]string),
	}

	if err := m.store.SaveSkill(ctx, skill); err != nil {
		return nil, err
	}

	return skill, nil
}

// AlignResult contains the results of an alignment operation
type AlignResult struct {
	Issues     []*analyzer.AlignmentIssue
	FixedCount int
	AllHealthy bool
	Report     *AlignReport
}

type AlignReport struct {
	AgentDirs []string
	Actions   []*AlignAction
}

type AlignAction struct {
	AgentDir  string
	SkillName string
	Action    string // link_created | skipped | error
	Success   bool
	Reason    string
}

// AlignAgents checks for alignment issues and optionally fixes them
func (m *Manager) AlignAgents(ctx context.Context, autoFix bool) (*AlignResult, error) {
	skills, err := m.List(ctx)
	if err != nil {
		return nil, err
	}

	// Analyze all skills
	issues := m.analyzer.AnalyzeAll(ctx, skills)

	// Align skills across locally-installed agents (filesystem-level alignment).
	fsIssues, fsFixed, fsReport, err := m.alignLocalAgents(ctx, autoFix)
	if err != nil {
		return nil, err
	}
	issues = append(issues, fsIssues...)

	result := &AlignResult{
		Issues:     issues,
		AllHealthy: func() bool {
			critical, _, _ := analyzer.GetSummary(issues)
			return critical == 0
		}(),
		Report: fsReport,
	}

	// Try to fix issues if requested
	if autoFix {
		for _, issue := range issues {
			if issue.Severity == analyzer.SeverityInfo {
				continue
			}

			// Find the skill
			var skill *models.Skill
			for _, s := range skills {
				if s.ID == issue.SkillID {
					skill = s
					break
				}
			}

			if skill == nil {
				continue
			}

			// Try to fix
			if err := m.fixIssue(ctx, skill, issue); err == nil {
				issue.Fixed = true
				result.FixedCount++
			}
		}
	}

	result.FixedCount += fsFixed
	return result, nil
}

func (m *Manager) alignLocalAgents(ctx context.Context, autoFix bool) ([]*analyzer.AlignmentIssue, int, *AlignReport, error) {
	sourceDir := m.cfg.Skill.SkillsDir
	sourceSkills, err := m.listSkillDirs(sourceDir)
	if err != nil {
		return nil, 0, nil, err
	}

	agentDirs := m.detectAgentDirs()
	report := &AlignReport{
		AgentDirs: agentDirs,
	}
	if len(agentDirs) == 0 {
		return []*analyzer.AlignmentIssue{{
			SkillID:     "",
			SkillName:   "",
			IssueType:   analyzer.IssueTypeAgentDirMissing,
			Description: "No agent skill directories detected. Configure skill.agent_dirs or create a supported agent skills directory.",
			Severity:    analyzer.SeverityCritical,
		}}, 0, report, nil
	}

	var issues []*analyzer.AlignmentIssue
	fixed := 0

	for _, ad := range agentDirs {
		if _, err := os.Stat(ad); err != nil {
			issues = append(issues, &analyzer.AlignmentIssue{
				IssueType:   analyzer.IssueTypeAgentDirMissing,
				Description: fmt.Sprintf("Agent skills directory missing: %s", ad),
				Severity:    analyzer.SeverityWarning,
			})
			report.Actions = append(report.Actions, &AlignAction{
				AgentDir: ad,
				Action:   "skipped",
				Success:  false,
				Reason:   "agent directory missing",
			})
			continue
		}

		for _, skillName := range sourceSkills {
			srcPath := filepath.Join(sourceDir, skillName)
			linkPath := filepath.Join(ad, skillName)

			fi, err := os.Lstat(linkPath)
			if err != nil {
				if os.IsNotExist(err) {
					issues = append(issues, &analyzer.AlignmentIssue{
						SkillName:   skillName,
						IssueType:   analyzer.IssueTypeSkillMissingInAgent,
						Description: fmt.Sprintf("Skill %q missing in agent dir %s", skillName, ad),
						Severity:    analyzer.SeverityWarning,
					})
					if autoFix {
						_ = os.MkdirAll(ad, 0755)
						if err := os.Symlink(srcPath, linkPath); err == nil {
							fixed++
							report.Actions = append(report.Actions, &AlignAction{
								AgentDir:  ad,
								SkillName: skillName,
								Action:    "link_created",
								Success:   true,
							})
						} else {
							report.Actions = append(report.Actions, &AlignAction{
								AgentDir:  ad,
								SkillName: skillName,
								Action:    "error",
								Success:   false,
								Reason:    err.Error(),
							})
						}
					} else {
						report.Actions = append(report.Actions, &AlignAction{
							AgentDir:  ad,
							SkillName: skillName,
							Action:    "skipped",
							Success:   false,
							Reason:    "missing (run with --fix to create symlink)",
						})
					}
				}
				continue
			}

			if fi.Mode()&os.ModeSymlink != 0 {
				target, err := os.Readlink(linkPath)
				if err != nil {
					issues = append(issues, &analyzer.AlignmentIssue{
						SkillName:   skillName,
						IssueType:   analyzer.IssueTypeSkillLinkMismatch,
						Description: fmt.Sprintf("Skill %q link unreadable in %s: %v", skillName, ad, err),
						Severity:    analyzer.SeverityWarning,
					})
					report.Actions = append(report.Actions, &AlignAction{
						AgentDir:  ad,
						SkillName: skillName,
						Action:    "skipped",
						Success:   false,
						Reason:    fmt.Sprintf("symlink unreadable: %v", err),
					})
					continue
				}
				// Allow relative/absolute differences by comparing cleaned absolute paths when possible.
				want := filepath.Clean(srcPath)
				got := filepath.Clean(target)
				if got != want {
					issues = append(issues, &analyzer.AlignmentIssue{
						SkillName:   skillName,
						IssueType:   analyzer.IssueTypeSkillLinkMismatch,
						Description: fmt.Sprintf("Skill %q in %s points to %s, expected %s", skillName, ad, got, want),
						Severity:    analyzer.SeverityWarning,
					})
					report.Actions = append(report.Actions, &AlignAction{
						AgentDir:  ad,
						SkillName: skillName,
						Action:    "skipped",
						Success:   false,
						Reason:    fmt.Sprintf("symlink points elsewhere: %s", got),
					})
				}
				continue
			}

			// Exists but is not a symlink (directory or file). We warn but don't touch it automatically.
			issues = append(issues, &analyzer.AlignmentIssue{
				SkillName:   skillName,
				IssueType:   analyzer.IssueTypeSkillLinkMismatch,
				Description: fmt.Sprintf("Skill %q exists in %s but is not a symlink (manual management?)", skillName, ad),
				Severity:    analyzer.SeverityWarning,
			})
			report.Actions = append(report.Actions, &AlignAction{
				AgentDir:  ad,
				SkillName: skillName,
				Action:    "skipped",
				Success:   false,
				Reason:    "exists but is not a symlink",
			})
		}
	}

	return issues, fixed, report, nil
}

func (m *Manager) detectAgentDirs() []string {
	// If configured explicitly, use it.
	if len(m.cfg.Skill.AgentDirs) > 0 {
		return m.cfg.Skill.AgentDirs
	}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".agent", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".hermes", "skills"),
		filepath.Join(home, ".openclaw", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".cursor", "skills-cursor"),
		// Common alternative locations used by some toolchains
		filepath.Join(home, ".config", "skills"),
	}

	var out []string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func (m *Manager) listSkillDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		// Valid skill dir: has SKILL.md/skill.json (same logic as scanner).
		if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err == nil {
			names = append(names, e.Name())
			continue
		}
		if _, err := os.Stat(filepath.Join(p, "skill.md")); err == nil {
			names = append(names, e.Name())
			continue
		}
		if _, err := os.Stat(filepath.Join(p, "skill.json")); err == nil {
			names = append(names, e.Name())
			continue
		}
	}
	sort.Strings(names)
	return names, nil
}

func (m *Manager) fixIssue(ctx context.Context, skill *models.Skill, issue *analyzer.AlignmentIssue) error {
	switch issue.IssueType {
	case analyzer.IssueTypeProcessZombie:
		// Process is gone, update status
		if err := m.store.UpdateSkillStatus(ctx, skill.ID, models.SkillStatusStopped); err != nil {
			return err
		}
		if err := m.store.UpdateSkillProcess(ctx, skill.ID, nil); err != nil {
			return err
		}
		return nil

	case analyzer.IssueTypeProcessMismatch:
		// Process running but marked as stopped - kill it
		return m.analyzer.FixIssue(ctx, skill, issue)

	case analyzer.IssueTypePortConflict:
		// PID is gone (or unknown), but port is still occupied by someone else.
		// Safe fix: clear hub state; do NOT attempt to kill the other process.
		if err := m.store.UpdateSkillStatus(ctx, skill.ID, models.SkillStatusStopped); err != nil {
			return err
		}
		if err := m.store.UpdateSkillProcess(ctx, skill.ID, nil); err != nil {
			return err
		}
		return nil

	case analyzer.IssueTypeHealthCheckFail:
		// Try to restart
		if err := m.Stop(ctx, skill.ID); err != nil {
			return err
		}
		return m.Start(ctx, skill.ID)

	default:
		return fmt.Errorf("unsupported issue type")
	}
}
