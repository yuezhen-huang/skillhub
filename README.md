# Skill Hub

Multi AI-agent skill management tool.

## Features

- Add/remove runnable skills from Git repositories (Go-based)
- Version management (branch/tag/commit switching)
- Skills run as independent processes with gRPC communication
- SQLite metadata storage
- Scan/import directory-based skills (e.g. `SKILL.md`) from a local skills directory
- Align skills across locally installed agents (filesystem sync via symlinks)
- Loose alignment checks (process/health/info) with optional safe auto-fix

## Quick Start

### Build

```bash
make build
```

### Start the daemon

```bash
bin/skillhub daemon
```

The daemon will automatically:

- scan the configured skills directory and import skills into SQLite
- run an alignment check and auto-fix safe issues

### Add a skill

```bash
bin/skillctl add my-skill --gitlab https://gitlab.com/example/skill.git --ref main
```

Add with config parameters (repeatable `--config`):

```bash
bin/skillctl add my-skill --gitlab https://gitlab.com/example/skill.git --ref main \
  --config foo=bar --config empty=
```

### List skills

```bash
bin/skillctl list
```

### Start a skill

```bash
bin/skillctl start <skill-id>
```

Note: documentation-only skills imported from a directory (e.g. `SKILL.md`) cannot be started as processes.

### Switch versions

```bash
bin/skillctl switch <skill-id> v1.0.0
bin/skillctl switch <skill-id> feature-branch
```

### Scan and import skills from a directory

Scan the configured skills directory (`skill.skills_dir`):

```bash
bin/skillctl scan
```

Scan and import all unimported skills:

```bash
bin/skillctl scan --import
```

Notes:

- A directory is considered a valid skill if it contains `SKILL.md` (or `skill.json`).
- Imported directory-based skills are documentation-only and cannot be started as processes.

### Agent alignment (sync skills across agents)

`align` treats `skill.skills_dir` as the **source-of-truth** skill set and aligns locally installed agents' skill directories to match it.

Alignment strategy:

- For each agent skills directory, if a skill is missing, `align --fix` will create a **symlink** pointing to the source skill directory.
- If a skill already exists but is **not a symlink**, it is reported as a mismatch and is **not modified automatically** (to avoid data loss).

Run alignment checks:

```bash
bin/skillctl align
```

Attempt safe auto-fixes:

```bash
bin/skillctl align --fix
```

Notes:

- `AllHealthy` is **true when there are no critical issues**. Warnings and infos are still shown.
- Auto-fix is **conservative**: it will not rename skills, overwrite non-symlink directories, or kill unknown processes occupying ports.
- `align --fix` prints an **execution report**: detected agent dirs, actions succeeded/failed, and reasons for skipped items.

By default, Skill Hub auto-detects common agent skill directories such as:

- `~/.agent/skills`
- `~/.agents/skills`
- `~/.claude/skills`
- `~/.hermes/skills`
- `~/.openclaw/skills`
- `~/.cursor/skills`
- `~/.cursor/skills-cursor`

You can override agent directory detection by providing `skill.agent_dirs` in your config file.

### Configuration (example)

Create `config.yaml`:

```yaml
hub:
  grpc_addr: ":50051"

storage:
  type: "sqlite"
  path: "~/.skillhub/skillhub.db"

skill:
  # Source-of-truth skill directory (scan/import and align use this)
  skills_dir: "~/.skillhub/skills"

  # Optional: explicitly list agent skill directories to align (overrides auto-detection)
  # agent_dirs:
  #   - "~/.claude/skills"
  #   - "~/.hermes/skills"

  port_start: 51000
  port_end: 52000
  build_timeout: 300

log:
  level: "info"
  format: "text"
```

Start daemon with config:

```bash
bin/skillhub daemon --config ./config.yaml
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

	"github.com/yuezhen-huang/skillhub/pkg/skillkit"
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
