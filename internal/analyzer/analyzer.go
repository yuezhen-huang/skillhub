package analyzer

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/yuezhen-huang/skillhub/internal/models"
	sk "github.com/yuezhen-huang/skillhub/internal/skill"
)

// IssueType represents the type of alignment issue
type IssueType string

const (
	IssueTypeProcessZombie    IssueType = "process_zombie"
	IssueTypeProcessMismatch  IssueType = "process_mismatch"
	IssueTypePortConflict     IssueType = "port_conflict"
	IssueTypeHealthCheckFail  IssueType = "health_check_fail"
	IssueTypeMissingBinary    IssueType = "missing_binary"
	IssueTypeStatusMismatch   IssueType = "status_mismatch"
	IssueTypeAgentDirMissing  IssueType = "agent_dir_missing"
	IssueTypeSkillMissingInAgent IssueType = "skill_missing_in_agent"
	IssueTypeSkillLinkMismatch IssueType = "skill_link_mismatch"
)

// IssueSeverity represents the severity of an issue
type IssueSeverity string

const (
	SeverityCritical IssueSeverity = "critical"
	SeverityWarning  IssueSeverity = "warning"
	SeverityInfo     IssueSeverity = "info"
)

// AlignmentIssue represents a detected alignment issue
type AlignmentIssue struct {
	SkillID     string        `json:"skill_id"`
	SkillName   string        `json:"skill_name"`
	IssueType   IssueType     `json:"issue_type"`
	Description string        `json:"description"`
	Severity    IssueSeverity `json:"severity"`
	Fixed       bool          `json:"fixed"`
}

// Analyzer handles agent alignment and health checking
type Analyzer struct {
	runtime *sk.Runtime
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(runtime *sk.Runtime) *Analyzer {
	return &Analyzer{
		runtime: runtime,
	}
}

// AnalyzeAll checks all skills for alignment issues
func (a *Analyzer) AnalyzeAll(ctx context.Context, skills []*models.Skill) []*AlignmentIssue {
	var issues []*AlignmentIssue

	for _, skill := range skills {
		skillIssues := a.AnalyzeSkill(ctx, skill)
		issues = append(issues, skillIssues...)
	}

	return issues
}

// AnalyzeSkill checks a single skill for alignment issues
func (a *Analyzer) AnalyzeSkill(ctx context.Context, skill *models.Skill) []*AlignmentIssue {
	var issues []*AlignmentIssue

	// Check status vs actual process
	issues = append(issues, a.checkProcessStatus(ctx, skill)...)

	// Check port conflicts (running, PID dead, but rpc_address still reachable)
	issues = append(issues, a.checkPortConflict(ctx, skill)...)

	// Check health/info if running and process appears alive
	if skill.Status == models.SkillStatusRunning && a.hasAliveProcess(skill) {
		issues = append(issues, a.checkHealth(ctx, skill)...)
		issues = append(issues, a.checkInfoAlignment(ctx, skill)...)
	}

	// Check binary exists
	if skill.Repository != nil && skill.Repository.Path != "" {
		issues = append(issues, a.checkBinary(skill)...)
	}

	return issues
}

func (a *Analyzer) hasAliveProcess(skill *models.Skill) bool {
	if skill == nil || skill.Process == nil || skill.Process.PID <= 0 {
		return false
	}
	return isProcessAlive(skill.Process.PID)
}

func (a *Analyzer) checkProcessStatus(ctx context.Context, skill *models.Skill) []*AlignmentIssue {
	var issues []*AlignmentIssue

	process := skill.Process

	switch skill.Status {
	case models.SkillStatusRunning:
		if process == nil || process.PID == 0 {
			issues = append(issues, &AlignmentIssue{
				SkillID:     skill.ID,
				SkillName:   skill.Name,
				IssueType:   IssueTypeStatusMismatch,
				Description: "Skill marked as running but no process info",
				Severity:    SeverityWarning,
			})
		} else if !isProcessAlive(process.PID) {
			issues = append(issues, &AlignmentIssue{
				SkillID:     skill.ID,
				SkillName:   skill.Name,
				IssueType:   IssueTypeProcessZombie,
				Description: fmt.Sprintf("Process PID %d not running but skill marked as running", process.PID),
				Severity:    SeverityCritical,
			})
		}

	case models.SkillStatusStopped, models.SkillStatusError:
		if process != nil && process.PID != 0 && isProcessAlive(process.PID) {
			issues = append(issues, &AlignmentIssue{
				SkillID:     skill.ID,
				SkillName:   skill.Name,
				IssueType:   IssueTypeProcessMismatch,
				Description: fmt.Sprintf("Process PID %d still running but skill marked as %s", process.PID, skill.Status),
				Severity:    SeverityWarning,
			})
		}
	}

	return issues
}

func (a *Analyzer) checkPortConflict(ctx context.Context, skill *models.Skill) []*AlignmentIssue {
	if skill == nil || skill.Status != models.SkillStatusRunning || skill.Process == nil {
		return nil
	}
	if skill.Process.PID <= 0 || strings.TrimSpace(skill.Process.RPCAddress) == "" {
		return nil
	}
	if isProcessAlive(skill.Process.PID) {
		return nil
	}

	d := net.Dialer{Timeout: 200 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", skill.Process.RPCAddress)
	if err != nil {
		return nil
	}
	_ = conn.Close()

	return []*AlignmentIssue{{
		SkillID:     skill.ID,
		SkillName:   skill.Name,
		IssueType:   IssueTypePortConflict,
		Description: fmt.Sprintf("PID %d not running but rpc_address %s is reachable; port likely occupied by another process", skill.Process.PID, skill.Process.RPCAddress),
		Severity:    SeverityWarning,
	}}
}

func (a *Analyzer) checkHealth(ctx context.Context, skill *models.Skill) []*AlignmentIssue {
	var issues []*AlignmentIssue

	healthy, err := a.runtime.HealthCheck(ctx, skill)
	if err != nil {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skill.ID,
			SkillName:   skill.Name,
			IssueType:   IssueTypeHealthCheckFail,
			Description: fmt.Sprintf("Health check failed: %v", err),
			Severity:    SeverityCritical,
		})
	} else if !healthy {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skill.ID,
			SkillName:   skill.Name,
			IssueType:   IssueTypeHealthCheckFail,
			Description: "Skill reported unhealthy",
			Severity:    SeverityWarning,
		})
	}

	return issues
}

func (a *Analyzer) checkInfoAlignment(ctx context.Context, skillModel *models.Skill) []*AlignmentIssue {
	if skillModel == nil || skillModel.Process == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	client, err := a.runtime.GetRPCClient(ctx, skillModel)
	if err != nil {
		return []*AlignmentIssue{{
			SkillID:     skillModel.ID,
			SkillName:   skillModel.Name,
			IssueType:   IssueTypeHealthCheckFail,
			Description: fmt.Sprintf("Info client setup failed: %v", err),
			Severity:    SeverityWarning,
		}}
	}
	defer client.Close()

	resp, err := client.Info(ctx)
	if err != nil {
		return []*AlignmentIssue{{
			SkillID:     skillModel.ID,
			SkillName:   skillModel.Name,
			IssueType:   IssueTypeHealthCheckFail,
			Description: fmt.Sprintf("Info call failed: %v", err),
			Severity:    SeverityWarning,
		}}
	}

	var issues []*AlignmentIssue

	// Name checks (loose mode: warning/info only, no auto-fix).
	if strings.TrimSpace(resp.Name) == "" {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skillModel.ID,
			SkillName:   skillModel.Name,
			IssueType:   IssueTypeStatusMismatch,
			Description: "Info.name is empty",
			Severity:    SeverityInfo,
		})
	} else if resp.Name != skillModel.Name {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skillModel.ID,
			SkillName:   skillModel.Name,
			IssueType:   IssueTypeStatusMismatch,
			Description: fmt.Sprintf("Info.name mismatch: hub=%q info=%q", skillModel.Name, resp.Name),
			Severity:    SeverityWarning,
		})
	}

	// Version checks (loose mode).
	infoVer := strings.TrimSpace(resp.Version)
	if infoVer == "" {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skillModel.ID,
			SkillName:   skillModel.Name,
			IssueType:   IssueTypeStatusMismatch,
			Description: "Info.version is empty",
			Severity:    SeverityInfo,
		})
	} else if looksLikeCommit(infoVer) {
		want := skillModel.Version
		got := infoVer
		if len(want) > 8 {
			want = want[:8]
		}
		if len(got) > 8 {
			got = got[:8]
		}
		if want != "" && got != "" && want != got {
			issues = append(issues, &AlignmentIssue{
				SkillID:     skillModel.ID,
				SkillName:   skillModel.Name,
				IssueType:   IssueTypeStatusMismatch,
				Description: fmt.Sprintf("Info.version (commit) mismatch: hub=%q info=%q", want, got),
				Severity:    SeverityWarning,
			})
		}
	}

	return issues
}

func looksLikeCommit(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) < 8 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func (a *Analyzer) checkBinary(skill *models.Skill) []*AlignmentIssue {
	var issues []*AlignmentIssue

	if skill.Repository == nil || skill.Repository.Path == "" {
		return issues
	}

	// Check if repository path exists
	if _, err := os.Stat(skill.Repository.Path); os.IsNotExist(err) {
		issues = append(issues, &AlignmentIssue{
			SkillID:     skill.ID,
			SkillName:   skill.Name,
			IssueType:   IssueTypeMissingBinary,
			Description: fmt.Sprintf("Repository path %s does not exist", skill.Repository.Path),
			Severity:    SeverityCritical,
		})
	}

	return issues
}

// FixIssue attempts to fix an alignment issue
func (a *Analyzer) FixIssue(ctx context.Context, skill *models.Skill, issue *AlignmentIssue) error {
	switch issue.IssueType {
	case IssueTypeProcessZombie:
		// Mark as stopped
		issue.Fixed = true
		return nil

	case IssueTypeProcessMismatch:
		if skill.Process != nil && isProcessAlive(skill.Process.PID) {
			// Try to kill it gently first
			if err := syscall.Kill(skill.Process.PID, syscall.SIGTERM); err == nil {
				issue.Fixed = true
				return nil
			}
			// Force kill
			if err := syscall.Kill(skill.Process.PID, syscall.SIGKILL); err == nil {
				issue.Fixed = true
				return nil
			}
		}
		issue.Fixed = true
		return nil

	default:
		return fmt.Errorf("cannot automatically fix issue type: %s", issue.IssueType)
	}
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	// On Unix systems, sending signal 0 checks if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// GetSummary returns a summary of alignment issues
func GetSummary(issues []*AlignmentIssue) (critical int, warnings int, info int) {
	for _, issue := range issues {
		switch issue.Severity {
		case SeverityCritical:
			critical++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			info++
		}
	}
	return critical, warnings, info
}
