package skill

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/yuezhen-huang/skillhub/api/gen/skill"
	"github.com/yuezhen-huang/skillhub/internal/models"
	"github.com/yuezhen-huang/skillhub/pkg/config"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Runtime manages skill processes
type Runtime struct {
	cfg       *config.SkillConfig
	processes map[string]*Process
	mu        sync.RWMutex
}

// NewRuntime creates a new skill runtime
func NewRuntime(cfg *config.SkillConfig) *Runtime {
	return &Runtime{
		cfg:       cfg,
		processes: make(map[string]*Process),
	}
}

// Spawn starts a new skill process
func (r *Runtime) Spawn(ctx context.Context, skillModel *models.Skill) (*models.ProcessInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.processes[skillModel.ID]; ok {
		if existing.IsRunning() {
			return nil, fmt.Errorf("skill %s is already running", skillModel.ID)
		}
	}

	port, err := r.findFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find free port: %w", err)
	}

	// Build the skill first
	binaryPath, err := Build(ctx, skillModel.Repository.Path, skillModel.ID)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	// Create process
	process := NewProcess(skillModel.ID, binaryPath, port, skillModel.Config)

	// Start the process
	if err := process.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// Wait for it to be ready
	if err := r.waitForReady(ctx, port, 10*time.Second); err != nil {
		process.Stop()
		return nil, fmt.Errorf("skill not ready: %w", err)
	}

	r.processes[skillModel.ID] = process

	return &models.ProcessInfo{
		PID:        process.PID(),
		Port:       port,
		RPCAddress: fmt.Sprintf("localhost:%d", port),
		StartedAt:  time.Now(),
	}, nil
}

// Kill stops a running skill process
func (r *Runtime) Kill(ctx context.Context, skillModel *models.Skill) error {
	// First try to stop tracked process (same daemon lifecycle).
	var (
		tracked *Process
		ok      bool
		pid     int
		id      string
	)

	r.mu.Lock()
	id = skillModel.ID
	tracked, ok = r.processes[id]
	// Capture persisted PID for daemon-restart scenarios (process map is empty).
	if !ok && skillModel.Process != nil {
		pid = skillModel.Process.PID
	}
	r.mu.Unlock()

	if ok {
		if err := tracked.Stop(); err != nil {
			return err
		}
		r.mu.Lock()
		delete(r.processes, id)
		r.mu.Unlock()
		return nil
	}

	// Fallback: daemon may have restarted, but storage still contains PID.
	if pid <= 0 {
		// Nothing to stop.
		return nil
	}

	return killPID(pid, 5*time.Second)
}

// HealthCheck checks if a skill is healthy
func (r *Runtime) HealthCheck(ctx context.Context, skillModel *models.Skill) (bool, error) {
	r.mu.RLock()
	process, ok := r.processes[skillModel.ID]
	r.mu.RUnlock()

	if !ok || !process.IsRunning() {
		return false, nil
	}

	client, err := r.GetRPCClient(ctx, skillModel)
	if err != nil {
		return false, err
	}
	defer client.Close()

	resp, err := client.HealthCheck(ctx)
	if err != nil {
		return false, err
	}

	return resp.Healthy, nil
}

// GetRPCClient gets a gRPC client for a skill
func (r *Runtime) GetRPCClient(ctx context.Context, skillModel *models.Skill) (*RPCClient, error) {
	if skillModel.Process == nil {
		return nil, fmt.Errorf("skill has no process info")
	}

	return NewRPCClient(skillModel.Process.RPCAddress)
}

// Cleanup stops all running processes
func (r *Runtime) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, process := range r.processes {
		process.Stop()
		delete(r.processes, id)
	}
}

func (r *Runtime) findFreePort() (int, error) {
	for port := r.cfg.PortStart; port <= r.cfg.PortEnd; port++ {
		addr := fmt.Sprintf(":%d", port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free ports available in range %d-%d", r.cfg.PortStart, r.cfg.PortEnd)
}

func (r *Runtime) waitForReady(ctx context.Context, port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	addr := fmt.Sprintf("localhost:%d", port)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conn, err := grpc.DialContext(ctx, addr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
				grpc.WithTimeout(500*time.Millisecond),
			)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

// Process represents a running skill process
type Process struct {
	id      string
	cmd     *exec.Cmd
	binary  string
	port    int
	config  map[string]string
	mu      sync.Mutex
	started bool
}

// NewProcess creates a new process wrapper
func NewProcess(id, binary string, port int, config map[string]string) *Process {
	return &Process{
		id:     id,
		binary: binary,
		port:   port,
		config: config,
	}
}

// Start starts the process
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("already started")
	}

	args := []string{
		"--port", fmt.Sprintf("%d", p.port),
	}
	for k, v := range p.config {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}

	cmd := exec.CommandContext(ctx, p.binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	p.cmd = cmd
	p.started = true

	return nil
}

// Stop stops the process
func (p *Process) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started || p.cmd == nil {
		return nil
	}

	if p.cmd.Process != nil {
		if err := p.cmd.Process.Signal(os.Interrupt); err == nil {
			// Wait a bit for graceful exit
			done := make(chan error, 1)
			go func() { done <- p.cmd.Wait() }()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				p.cmd.Process.Kill()
			}
		} else {
			p.cmd.Process.Kill()
		}
	}

	p.started = false
	return nil
}

// PID returns the process ID
func (p *Process) PID() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// IsRunning checks if the process is running
func (p *Process) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started || p.cmd == nil || p.cmd.Process == nil {
		return false
	}

	// Check if process is still alive
	if err := p.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	return true
}

// RPCClient wraps the gRPC skill client
type RPCClient struct {
	conn   *grpc.ClientConn
	client skill.SkillClient
}

// NewRPCClient creates a new RPC client
func NewRPCClient(addr string) (*RPCClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &RPCClient{
		conn:   conn,
		client: skill.NewSkillClient(conn),
	}, nil
}

// Info calls the Info RPC
func (c *RPCClient) Info(ctx context.Context) (*skill.InfoResponse, error) {
	return c.client.Info(ctx, &skill.InfoRequest{})
}

// Execute calls the Execute RPC
func (c *RPCClient) Execute(ctx context.Context, method string, payload []byte, metadata map[string]string) (*skill.ExecuteResponse, error) {
	return c.client.Execute(ctx, &skill.ExecuteRequest{
		Method:   method,
		Payload:  payload,
		Metadata: metadata,
	})
}

// HealthCheck calls the HealthCheck RPC
func (c *RPCClient) HealthCheck(ctx context.Context) (*skill.HealthCheckResponse, error) {
	return c.client.HealthCheck(ctx, &skill.HealthCheckRequest{})
}

// Close closes the client connection
func (c *RPCClient) Close() error {
	return c.conn.Close()
}

// Build builds a skill from source
func Build(ctx context.Context, srcPath, id string) (string, error) {
	// Check if there's a main package in the root or cmd/skill directory
	var mainDir string
	for _, dir := range []string{".", "cmd/skill", "cmd", "main"} {
		if hasMainGo(filepath.Join(srcPath, dir)) {
			mainDir = dir
			break
		}
	}

	if mainDir == "" {
		return "", fmt.Errorf("no main package found in %s", srcPath)
	}

	buildDir := filepath.Join(srcPath, ".build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", err
	}

	binaryPath := filepath.Join(buildDir, id)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./"+mainDir)
	cmd.Dir = srcPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build failed: %w", err)
	}

	return binaryPath, nil
}

func hasMainGo(dir string) bool {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return false
	}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		if len(content) > 0 && contains(string(content), "package main") && contains(string(content), "func main") {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || indexOf(s, substr) != -1)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Unix systems, signal 0 checks for existence/permission.
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

func killPID(pid int, timeout time.Duration) error {
	if !isPIDAlive(pid) {
		return nil
	}

	// Try graceful termination first.
	_ = syscall.Kill(pid, syscall.SIGTERM)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !isPIDAlive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill.
	_ = syscall.Kill(pid, syscall.SIGKILL)
	return nil
}
