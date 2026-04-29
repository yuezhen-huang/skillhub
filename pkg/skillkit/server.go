package skillkit

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/skillhub/skill-hub/api/gen/skill"

	"google.golang.org/grpc"
)

// Server runs the gRPC server for a skill
type Server struct {
	skill.UnimplementedSkillServer

	skillImpl Skill
	port      int
	server    *grpc.Server
	mu        sync.Mutex
}

// NewServer creates a new skill server
func NewServer(skillImpl Skill, port int) *Server {
	return &Server{
		skillImpl: skillImpl,
		port:      port,
	}
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return fmt.Errorf("server already running")
	}

	addr := fmt.Sprintf(":%d", s.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	skill.RegisterSkillServer(s.server, s)

	go func() {
		if err := s.server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			fmt.Printf("server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		s.server.GracefulStop()
		s.server = nil
	}
}

// Info implements the Info RPC
func (s *Server) Info(ctx context.Context, req *skill.InfoRequest) (*skill.InfoResponse, error) {
	info, err := s.skillImpl.Info(ctx)
	if err != nil {
		return nil, err
	}

	return &skill.InfoResponse{
		Name:        info.Name,
		Version:     info.Version,
		Description: info.Description,
	}, nil
}

// Execute implements the Execute RPC
func (s *Server) Execute(ctx context.Context, req *skill.ExecuteRequest) (*skill.ExecuteResponse, error) {
	resp, err := s.skillImpl.Execute(ctx, &ExecuteRequest{
		Method:   req.Method,
		Payload:  req.Payload,
		Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}

	if resp == nil {
		return &skill.ExecuteResponse{}, nil
	}

	return &skill.ExecuteResponse{
		Result:   resp.Result,
		Metadata: resp.Metadata,
	}, nil
}

// HealthCheck implements the HealthCheck RPC
func (s *Server) HealthCheck(ctx context.Context, req *skill.HealthCheckRequest) (*skill.HealthCheckResponse, error) {
	return &skill.HealthCheckResponse{
		Healthy: true,
		Message: "ok",
	}, nil
}
