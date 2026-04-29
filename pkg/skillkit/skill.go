package skillkit

import (
	"context"
)

// Skill defines the interface that skills must implement
type Skill interface {
	// Info returns skill information
	Info(ctx context.Context) (*Info, error)

	// Execute executes a method on the skill
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
}

// Info contains skill metadata
type Info struct {
	Name        string
	Version     string
	Description string
}

// ExecuteRequest is a request to execute a skill method
type ExecuteRequest struct {
	Method   string
	Payload  []byte
	Metadata map[string]string
}

// ExecuteResponse is the response from executing a skill method
type ExecuteResponse struct {
	Result   []byte
	Metadata map[string]string
}

// BaseSkill provides a base implementation with common functionality
type BaseSkill struct {
	info Info
}

// NewBaseSkill creates a new base skill
func NewBaseSkill(name, version, description string) *BaseSkill {
	return &BaseSkill{
		info: Info{
			Name:        name,
			Version:     version,
			Description: description,
		},
	}
}

// Info returns the skill information
func (b *BaseSkill) Info(ctx context.Context) (*Info, error) {
	return &b.info, nil
}

// Execute is a placeholder - override in your skill
func (b *BaseSkill) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return nil, nil
}
