# Skill Hub

Multi AI-agent skill management tool.

## Features

- Add/remove skills from GitLab repositories
- Version management (branch/tag/commit switching)
- Skills run as independent processes with gRPC communication
- SQLite metadata storage

## Quick Start

### Build

```bash
make build
```

### Start the daemon

```bash
bin/skillhub daemon
```

### Add a skill

```bash
bin/skillctl add my-skill --gitlab https://gitlab.com/example/skill.git --branch main
```

### List skills

```bash
bin/skillctl list
```

### Start a skill

```bash
bin/skillctl start <skill-id>
```

### Switch versions

```bash
bin/skillctl switch <skill-id> v1.0.0
bin/skillctl switch <skill-id> feature-branch
```

### Stop and remove

```bash
bin/skillctl stop <skill-id>
bin/skillctl remove <skill-id>
```

## Skill Development

Skills are Go packages that implement the `skillkit.Skill` interface.

### Example skill

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/skillhub/skill-hub/pkg/skillkit"
)

type MySkill struct {
	*skillkit.BaseSkill
}

func NewMySkill() *MySkill {
	return &MySkill{
		BaseSkill: skillkit.NewBaseSkill("my-skill", "1.0.0", "My example skill"),
	}
}

func (s *MySkill) Execute(ctx context.Context, req *skillkit.ExecuteRequest) (*skillkit.ExecuteResponse, error) {
	switch req.Method {
	case "hello":
		return &skillkit.ExecuteResponse{
			Result: []byte("Hello!"),
		}, nil
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

func main() {
	var port int
	flag.IntVar(&port, "port", 50052, "Port to listen on")
	flag.Parse()

	skill := NewMySkill()
	server := skillkit.NewServer(skill, port)

	if err := server.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	server.Stop()
}
```

## Project Structure

```
├── api/
│   ├── proto/          # Protobuf definitions
│   └── gen/            # Generated gRPC code
├── cmd/
│   ├── skillhub/       # Hub daemon
│   └── skillctl/       # CLI tool
├── internal/
│   ├── hub/            # Hub core logic
│   ├── skill/          # Skill runtime management
│   ├── gitlab/         # Git repository operations
│   ├── storage/        # SQLite storage
│   └── models/         # Data models
└── pkg/
    └── skillkit/       # Skill development SDK
```
