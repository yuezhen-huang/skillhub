package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/skillhub/skill-hub/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS skills (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		version TEXT,
		description TEXT,
		status TEXT NOT NULL,
		repository_json TEXT,
		process_json TEXT,
		config_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`

	_, err := s.db.Exec(schema)
	return err
}

// SaveSkill saves a skill to storage
func (s *SQLiteStore) SaveSkill(ctx context.Context, skill *models.Skill) error {
	repoJSON, err := json.Marshal(skill.Repository)
	if err != nil {
		return err
	}

	processJSON, err := json.Marshal(skill.Process)
	if err != nil {
		return err
	}

	configJSON, err := json.Marshal(skill.Config)
	if err != nil {
		return err
	}

	now := time.Now()
	if skill.CreatedAt.IsZero() {
		skill.CreatedAt = now
	}
	skill.UpdatedAt = now

	_, err = s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO skills
		(id, name, version, description, status, repository_json, process_json, config_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, skill.ID, skill.Name, skill.Version, skill.Description, skill.Status, repoJSON, processJSON, configJSON, skill.CreatedAt, skill.UpdatedAt)

	return err
}

// GetSkill retrieves a skill by ID
func (s *SQLiteStore) GetSkill(ctx context.Context, id string) (*models.Skill, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, description, status, repository_json, process_json, config_json, created_at, updated_at
		FROM skills WHERE id = ?
	`, id)

	return s.scanSkill(row)
}

// GetSkillByName retrieves a skill by name
func (s *SQLiteStore) GetSkillByName(ctx context.Context, name string) (*models.Skill, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, version, description, status, repository_json, process_json, config_json, created_at, updated_at
		FROM skills WHERE name = ?
	`, name)

	return s.scanSkill(row)
}

// ListSkills lists all skills
func (s *SQLiteStore) ListSkills(ctx context.Context) ([]*models.Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, version, description, status, repository_json, process_json, config_json, created_at, updated_at
		FROM skills ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []*models.Skill
	for rows.Next() {
		skill, err := s.scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}

	return skills, rows.Err()
}

// DeleteSkill deletes a skill by ID
func (s *SQLiteStore) DeleteSkill(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM skills WHERE id = ?", id)
	return err
}

// UpdateSkillStatus updates a skill's status
func (s *SQLiteStore) UpdateSkillStatus(ctx context.Context, id string, status models.SkillStatus) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE skills SET status = ?, updated_at = ? WHERE id = ?
	`, status, time.Now(), id)
	return err
}

// UpdateSkillProcess updates a skill's process info
func (s *SQLiteStore) UpdateSkillProcess(ctx context.Context, id string, process *models.ProcessInfo) error {
	processJSON, err := json.Marshal(process)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE skills SET process_json = ?, updated_at = ? WHERE id = ?
	`, processJSON, time.Now(), id)
	return err
}

// UpdateSkillRepository updates a skill's repository info
func (s *SQLiteStore) UpdateSkillRepository(ctx context.Context, id string, repo *models.Repository) error {
	repoJSON, err := json.Marshal(repo)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE skills SET repository_json = ?, updated_at = ? WHERE id = ?
	`, repoJSON, time.Now(), id)
	return err
}

// Close closes the storage connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStore) scanSkill(sc scanner) (*models.Skill, error) {
	var (
		skill          models.Skill
		repoJSON       sql.NullString
		processJSON    sql.NullString
		configJSON     sql.NullString
	)

	err := sc.Scan(
		&skill.ID, &skill.Name, &skill.Version, &skill.Description, &skill.Status,
		&repoJSON, &processJSON, &configJSON, &skill.CreatedAt, &skill.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found")
		}
		return nil, err
	}

	if repoJSON.Valid {
		var repo models.Repository
		if err := json.Unmarshal([]byte(repoJSON.String), &repo); err != nil {
			return nil, err
		}
		skill.Repository = &repo
	}

	if processJSON.Valid {
		var process models.ProcessInfo
		if err := json.Unmarshal([]byte(processJSON.String), &process); err != nil {
			return nil, err
		}
		skill.Process = &process
	}

	if configJSON.Valid {
		if err := json.Unmarshal([]byte(configJSON.String), &skill.Config); err != nil {
			return nil, err
		}
	}

	return &skill, nil
}

func ensureDir(path string) error {
	if path == "" || path == "." {
		return nil
	}
	return nil
}
