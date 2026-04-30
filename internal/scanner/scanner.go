package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuezhen-huang/skillhub/pkg/config"

	"github.com/go-git/go-git/v5"
)

// DiscoveredSkill represents a skill found during scanning
type DiscoveredSkill struct {
	Path            string            `json:"path"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	DetectedVersion string            `json:"detected_version"`
	AlreadyImported bool              `json:"already_imported"`
	IsValidSkill    bool              `json:"is_valid_skill"`
	ValidationError string            `json:"validation_error,omitempty"`
}

// Scanner handles scanning for existing skills
type Scanner struct {
	skillsDir string
}

// NewScanner creates a new scanner
func NewScanner(cfg *config.Config) *Scanner {
	return &Scanner{
		skillsDir: cfg.Skill.SkillsDir,
	}
}

// ScanSkillsDir scans the skills directory for existing skills
func (s *Scanner) ScanSkillsDir(ctx context.Context, existingSkills map[string]bool) ([]*DiscoveredSkill, error) {
	if _, err := os.Stat(s.skillsDir); os.IsNotExist(err) {
		return []*DiscoveredSkill{}, nil
	}

	entries, err := os.ReadDir(s.skillsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	var discovered []*DiscoveredSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(s.skillsDir, entry.Name())
		skill := s.analyzeSkillDir(ctx, skillPath, existingSkills)
		discovered = append(discovered, skill)
	}

	return discovered, nil
}

func (s *Scanner) analyzeSkillDir(ctx context.Context, path string, existingSkills map[string]bool) *DiscoveredSkill {
	result := &DiscoveredSkill{
		Path:         path,
		Name:         filepath.Base(path),
		IsValidSkill: true,
	}

	// Try to detect skill name
	dirName := result.Name
	result.Name = s.detectSkillName(path)

	// Check if already imported (support both detected name and directory name)
	result.AlreadyImported = existingSkills[result.Name] || existingSkills[dirName]

	// Validate it looks like a "skill directory".
	// Requirement: the directory can contain documentation-only skills (e.g. SKILL.md) and does not have to be a git/go project.
	if !s.looksLikeSkillDir(path) {
		result.IsValidSkill = false
		result.ValidationError = "missing SKILL.md (or skill.json)"
		return result
	}

	// Try to get commit info if it happens to be a git repo (optional).
	if repo, err := git.PlainOpen(path); err == nil {
		if head, err := repo.Head(); err == nil {
			result.DetectedVersion = head.Hash().String()
			if len(result.DetectedVersion) > 8 {
				result.DetectedVersion = result.DetectedVersion[:8]
			}
		}
	}

	// Try to find a skill manifest or readme for description
	result.Description = s.detectDescription(path)

	return result
}

func (s *Scanner) detectSkillName(path string) string {
	// Fall back to directory name
	return filepath.Base(path)
}

func (s *Scanner) detectDescription(path string) string {
	// Try SKILL.md first (common for directory-based skills)
	for _, name := range []string{"SKILL.md", "skill.md"} {
		skillPath := filepath.Join(path, name)
		if data, err := os.ReadFile(skillPath); err == nil {
			lines := strings.SplitN(string(data), "\n", 10)
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Prefer first heading text, else first non-empty line.
				line = strings.TrimPrefix(line, "#")
				line = strings.TrimSpace(line)
				if line != "" {
					if len(line) > 100 {
						line = line[:100] + "..."
					}
					return line
				}
			}
		}
	}

	// Try README.md
	for _, name := range []string{"README.md", "README", "readme.md", "readme"} {
		readmePath := filepath.Join(path, name)
		if data, err := os.ReadFile(readmePath); err == nil {
			lines := strings.SplitN(string(data), "\n", 3)
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					// Take first non-empty, non-heading line as description
					if len(line) > 100 {
						line = line[:100] + "..."
					}
					return line
				}
			}
		}
	}

	// Try skill manifest
	manifestPath := filepath.Join(path, "skill.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var manifest struct {
			Description string `json:"description"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			return manifest.Description
		}
	}

	return ""
}

func (s *Scanner) looksLikeSkillDir(path string) bool {
	// Documentation-based skill directory: SKILL.md or skill.json is enough.
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "skill.md")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, "skill.json")); err == nil {
		return true
	}
	return false
}