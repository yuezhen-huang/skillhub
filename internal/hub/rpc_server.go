package hub

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/skillhub/skill-hub/api/gen/skillhub"
	"github.com/skillhub/skill-hub/internal/models"

	"google.golang.org/grpc"
)

// RPCServer implements the SkillHub gRPC service
type RPCServer struct {
	skillhub.UnimplementedSkillHubServer

	manager *Manager
	addr    string
	server  *grpc.Server
	mu      sync.Mutex
}

// NewRPCServer creates a new RPC server
func NewRPCServer(manager *Manager, addr string) *RPCServer {
	return &RPCServer{
		manager: manager,
		addr:    addr,
	}
}

// Start starts the gRPC server
func (s *RPCServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return fmt.Errorf("server already running")
	}

	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	skillhub.RegisterSkillHubServer(s.server, s)

	go func() {
		if err := s.server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the gRPC server
func (s *RPCServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		s.server.GracefulStop()
		s.server = nil
	}
}

// AddSkill implements AddSkill RPC
func (s *RPCServer) AddSkill(ctx context.Context, req *skillhub.AddSkillRequest) (*skillhub.AddSkillResponse, error) {
	skill, err := s.manager.Add(ctx, &models.SkillSpec{
		Name:       req.Name,
		GitLabURL:  req.GitlabUrl,
		VersionRef: req.VersionRef,
		Config:     req.Config,
	})
	if err != nil {
		return nil, err
	}

	return &skillhub.AddSkillResponse{
		Skill: toProtoSkill(skill),
	}, nil
}

// RemoveSkill implements RemoveSkill RPC
func (s *RPCServer) RemoveSkill(ctx context.Context, req *skillhub.RemoveSkillRequest) (*skillhub.RemoveSkillResponse, error) {
	err := s.manager.Remove(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.RemoveSkillResponse{
		Success: true,
	}, nil
}

// GetSkill implements GetSkill RPC
func (s *RPCServer) GetSkill(ctx context.Context, req *skillhub.GetSkillRequest) (*skillhub.GetSkillResponse, error) {
	skill, err := s.manager.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.GetSkillResponse{
		Skill: toProtoSkill(skill),
	}, nil
}

// ListSkills implements ListSkills RPC
func (s *RPCServer) ListSkills(ctx context.Context, req *skillhub.ListSkillsRequest) (*skillhub.ListSkillsResponse, error) {
	skills, err := s.manager.List(ctx)
	if err != nil {
		return nil, err
	}

	protoSkills := make([]*skillhub.Skill, len(skills))
	for i, skill := range skills {
		protoSkills[i] = toProtoSkill(skill)
	}

	return &skillhub.ListSkillsResponse{
		Skills: protoSkills,
	}, nil
}

// StartSkill implements StartSkill RPC
func (s *RPCServer) StartSkill(ctx context.Context, req *skillhub.StartSkillRequest) (*skillhub.StartSkillResponse, error) {
	err := s.manager.Start(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.StartSkillResponse{
		Success: true,
	}, nil
}

// StopSkill implements StopSkill RPC
func (s *RPCServer) StopSkill(ctx context.Context, req *skillhub.StopSkillRequest) (*skillhub.StopSkillResponse, error) {
	err := s.manager.Stop(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.StopSkillResponse{
		Success: true,
	}, nil
}

// RestartSkill implements RestartSkill RPC
func (s *RPCServer) RestartSkill(ctx context.Context, req *skillhub.RestartSkillRequest) (*skillhub.RestartSkillResponse, error) {
	err := s.manager.Restart(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.RestartSkillResponse{
		Success: true,
	}, nil
}

// SwitchVersion implements SwitchVersion RPC
func (s *RPCServer) SwitchVersion(ctx context.Context, req *skillhub.SwitchVersionRequest) (*skillhub.SwitchVersionResponse, error) {
	err := s.manager.SwitchVersion(ctx, req.Id, req.VersionRef)
	if err != nil {
		return nil, err
	}

	skill, _ := s.manager.Get(ctx, req.Id)

	return &skillhub.SwitchVersionResponse{
		Success: true,
		Version: skill.Version,
	}, nil
}

// ListVersions implements ListVersions RPC
func (s *RPCServer) ListVersions(ctx context.Context, req *skillhub.ListVersionsRequest) (*skillhub.ListVersionsResponse, error) {
	tags, branches, err := s.manager.ListVersions(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.ListVersionsResponse{
		Tags:     tags,
		Branches: branches,
	}, nil
}

// PullLatest implements PullLatest RPC
func (s *RPCServer) PullLatest(ctx context.Context, req *skillhub.PullLatestRequest) (*skillhub.PullLatestResponse, error) {
	commit, err := s.manager.PullLatest(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &skillhub.PullLatestResponse{
		Success: true,
		Commit:  commit,
	}, nil
}

func toProtoSkill(skill *models.Skill) *skillhub.Skill {
	if skill == nil {
		return nil
	}

	var repo *skillhub.Repository
	if skill.Repository != nil {
		repo = &skillhub.Repository{
			Url:    skill.Repository.URL,
			Branch: skill.Repository.Branch,
			Tag:    skill.Repository.Tag,
			Commit: skill.Repository.Commit,
		}
	}

	var proc *skillhub.ProcessInfo
	if skill.Process != nil {
		proc = &skillhub.ProcessInfo{
			Pid:        int32(skill.Process.PID),
			RpcAddress: skill.Process.RPCAddress,
		}
	}

	return &skillhub.Skill{
		Id:         skill.ID,
		Name:       skill.Name,
		Version:    skill.Version,
		Status:     string(skill.Status),
		Repository: repo,
		Process:    proc,
	}
}
