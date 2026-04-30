# Skill Hub 技术方案文档

## 1. 项目概述

### 1.1 项目背景
Skill Hub 是一个多 AI-agent skill 管理工具，支持从 GitLab 仓库导入 skill，进行版本管理、生命周期控制和通信交互。

### 1.2 核心目标
- 支持 skill 的 Add/Remove 管理
- 集成 GitLab 作为 skill 源
- 提供分支/Tag/Commit 级别的版本管理
- 独立进程运行 skill，通过 gRPC 通信

## 2. 架构设计

### 2.1 系统架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        Skill Hub                             │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    gRPC     ┌──────────────────┐          │
│  │   skillctl   │◄──────────►│  skillhub daemon │          │
│  │   (CLI)      │            │  (gRPC Server)   │          │
│  └──────────────┘            └─────────┬────────┘          │
│                                       │                     │
│                              ┌────────▼────────┐            │
│                              │  Skill Manager │            │
│                              └────────┬────────┘            │
│         ┌────────────────────────────┼──────────────────┐  │
│         │                            │                  │  │
│  ┌──────▼──────┐              ┌──────▼─────┐    ┌───────▼────┐│
│  │  Storage    │              │  Runtime   │    │   GitLab   ││
│  │  (SQLite)   │              │  Manager   │    │  Manager   ││
│  └──────┬──────┘              └──────┬─────┘    └───────┬────┘│
│         │                            │                   │     │
└─────────┼────────────────────────────┼───────────────────┼─────┘
          │                            │                   │
          │              ┌─────────────┼──────────────┐   │
          │              │             │              │   │
          ▼              ▼             ▼              ▼   ▼
     ┌─────────┐   ┌──────────┐  ┌──────────┐   ┌─────────┐
     │ Skill 1 │   │ Skill 2  │  │ Skill 3  │   │  GitLab │
     │Process │   │ Process  │  │ Process  │  │ Remote │
     └─────────┘   └──────────┘  └──────────┘   └─────────┘
          │              │             │
          └──────────────┴─────────────┘
              gRPC Communication

```

### 2.2 分层架构

| 层级 | 职责 | 目录 |
|-----|------|-----|
| API 层 | gRPC 接口定义 | api/proto/ |
| 服务层 | Hub 服务实现 | internal/hub/ |
| 领域层 | 业务逻辑 | internal/gitlab/, internal/skill/ |
| 基础设施层 | 存储、配置 | internal/storage/, pkg/config/ |
| SDK 层 | Skill 开发工具包 | pkg/skillkit/ |
| 工具层 | CLI 和 Daemon | cmd/ |

## 3. 核心模块设计

### 3.1 数据模型

#### 3.1.1 Skill 模型

```go
type Skill struct {
    ID          string            // 唯一标识
    Name        string            // Skill 名称
    Version     string            // 当前版本 (commit hash 前8位)
    Description string            // 描述信息
    Repository  *Repository       // Git 仓库信息
    Status      SkillStatus       // 运行状态
    Process     *ProcessInfo      // 进程信息 (运行时)
    Config      map[string]string // 配置
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type SkillStatus string

const (
    SkillStatusUnknown  SkillStatus = "unknown"
    SkillStatusStopped  SkillStatus = "stopped"
    SkillStatusStarting SkillStatus = "starting"
    SkillStatusRunning  SkillStatus = "running"
    SkillStatusStopping SkillStatus = "stopping"
    SkillStatusError    SkillStatus = "error"
)
```

#### 3.1.2 Repository 模型

```go
type Repository struct {
    ID       string
    URL      string        // Git URL
    Remote   string        // Remote 名称 (默认 "origin")
    Path     string        // 本地存储路径
    Branch   string        // 当前分支
    Tag      string        // 当前 Tag (如有)
    Commit   string        // 当前 Commit hash
    LastPull *time.Time    // 最后 Pull 时间
}
```

#### 3.1.3 ProcessInfo 模型

```go
type ProcessInfo struct {
    PID        int       // 进程 ID
    Port       int       // gRPC 监听端口
    RPCAddress string    // gRPC 地址 (localhost:port)
    StartedAt  time.Time // 启动时间
}
```

### 3.2 GitLab 集成模块

#### 3.2.1 RepositoryManager 接口

```go
type RepositoryManager interface {
    // 克隆仓库
    Clone(ctx context.Context, url, path string) (*Repository, error)

    // 拉取最新变更
    Pull(ctx context.Context, repo *Repository) error

    // 切换分支
    CheckoutBranch(ctx context.Context, repo *Repository, branch string) error

    // 切换 Tag
    CheckoutTag(ctx context.Context, repo *Repository, tag string) error

    // 切换 Commit
    CheckoutCommit(ctx context.Context, repo *Repository, commit string) error

    // 列出所有 Tag
    ListTags(ctx context.Context, repo *Repository) ([]string, error)

    // 列出所有分支
    ListBranches(ctx context.Context, repo *Repository) ([]string, error)

    // 获取当前 Commit
    GetCurrentCommit(ctx context.Context, repo *Repository) (string, error)
}
```

#### 3.2.2 技术选型
- **go-git**: 纯 Go 实现的 Git 库，无需依赖系统 git
- **特性支持**: Clone, Pull, Fetch, Checkout (Branch/Tag/Commit)

### 3.3 Skill 运行时模块

#### 3.3.1 Runtime 职责
- 管理 skill 进程的生命周期
- 分配端口
- 构建 skill 二进制
- gRPC 客户端连接池

#### 3.3.2 进程管理流程

```
Start Skill
    │
    ▼
1. 查找空闲端口 (51000-52000)
    │
    ▼
2. 执行 Go Build 编译 skill
    │
    ▼
3. 启动子进程 (传递端口参数)
    │
    ▼
4. 等待 gRPC 服务就绪 (健康检查)
    │
    ▼
5. 更新状态为 Running
```

#### 3.3.3 Skill 构建
- 自动检测 main 包位置 (根目录、cmd/、cmd/skill/)
- 输出到 `.build/` 目录
- 使用 `go build` 编译

### 3.4 Hub 核心模块

#### 3.4.1 Manager 职责
- Skill CRUD 操作
- 状态机管理
- 版本切换协调
- 健康检查

#### 3.4.2 状态机流转

```
┌──────────┐     Start      ┌───────────┐
│ Stopped  │◄───────────────┤ Starting  │
└─────┬────┘                └─────┬─────┘
      │                           │
      │ Stop                      │ 就绪
      │                    ┌──────▼──────┐
      │                    │   Running   │
      │                    └──────┬──────┘
      │                           │
      │                    ┌──────▼──────┐
      └────────────────────┤  Stopping  │
                           └─────────────┘
                               │
                               ▼
                          ┌─────────┐
                          │  Error  │
                          └─────────┘
```

### 3.5 存储模块

#### 3.5.1 存储设计
- **数据库**: SQLite
- **存储内容**: Skill 元数据
- **Schema**: 单表 skills

#### 3.5.2 Schema 结构

```sql
CREATE TABLE skills (
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
);
```

#### 3.5.3 序列化策略
- Repository、ProcessInfo、Config 以 JSON 存储
- 使用 database/sql 标准库

## 4. gRPC API 设计

### 4.1 SkillHub 服务

```protobuf
service SkillHub {
  // Skill 管理
  rpc AddSkill(AddSkillRequest) returns (AddSkillResponse);
  rpc RemoveSkill(RemoveSkillRequest) returns (RemoveSkillResponse);
  rpc GetSkill(GetSkillRequest) returns (GetSkillResponse);
  rpc ListSkills(ListSkillsRequest) returns (ListSkillsResponse);

  // 生命周期控制
  rpc StartSkill(StartSkillRequest) returns (StartSkillResponse);
  rpc StopSkill(StopSkillRequest) returns (StopSkillResponse);
  rpc RestartSkill(RestartSkillRequest) returns (RestartSkillResponse);

  // 版本管理
  rpc SwitchVersion(SwitchVersionRequest) returns (SwitchVersionResponse);
  rpc ListVersions(ListVersionsRequest) returns (ListVersionsResponse);
  rpc PullLatest(PullLatestRequest) returns (PullLatestResponse);
}
```

### 4.2 Skill 服务

```protobuf
service Skill {
  rpc Info(InfoRequest) returns (InfoResponse);
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
  rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
}
```

### 4.3 API 详细说明

#### 4.3.1 AddSkill
- **请求**: name, gitlab_url, version_ref, config
- **响应**: 完整 Skill 信息
- **流程**: 验证 → Clone → Checkout → 保存

#### 4.3.2 SwitchVersion
- **请求**: skill_id, version_ref
- **响应**: success, new_version
- **流程**: 停止(如运行) → Fetch → Checkout → 更新版本

## 5. 技术选型与依赖

### 5.1 核心依赖

| 组件 | 库 | 版本 | 用途 |
|-----|----|------|-----|
| 语言 | Go | 1.21+ | 开发语言 |
| RPC | gRPC | 1.62+ | 服务通信 |
| Protobuf | protobuf | 最新 | 接口定义 |
| Git | go-git/v5 | 5.11+ | Git 操作 |
| 配置 | Viper | 1.19+ | 配置管理 |
| CLI | Cobra | 1.8+ | 命令行框架 |
| 存储 | go-sqlite3 | 1.14+ | SQLite 驱动 |
| UUID | google/uuid | 1.6+ | 唯一 ID 生成 |

### 5.2 依赖关系图

```
skillhub (main)
  ├─ internal/hub
  │   ├─ internal/models
  │   ├─ internal/gitlab (go-git)
  │   ├─ internal/skill
  │   └─ internal/storage (sqlite3)
  ├─ pkg/config (viper)
  └─ api/gen (protobuf)

skillctl (main)
  ├─ api/gen (protobuf)
  └─ github.com/spf13/cobra
```

## 6. 目录结构

```
skill-hub/
├── api/
│   ├── proto/
│   │   ├── skillhub.proto      # Hub gRPC 定义
│   │   └── skill.proto         # Skill gRPC 定义
│   └── gen/                     # 生成的 Go 代码
│       ├── skillhub/
│       └── skill/
├── cmd/
│   ├── skillhub/
│   │   └── main.go             # Hub 守护进程入口
│   └── skillctl/
│       └── main.go             # CLI 入口
├── internal/
│   ├── hub/
│   │   ├── manager.go          # Skill 管理器
│   │   ├── rpc_server.go       # gRPC 服务实现
│   │   └── registry.go         # Skill 注册表
│   ├── skill/
│   │   ├── runtime.go          # Skill 运行时
│   │   ├── process.go          # 进程包装
│   │   ├── builder.go          # Skill 构建器
│   │   └── client.go           # gRPC 客户端
│   ├── gitlab/
│   │   ├── repository.go       # Git 仓库操作
│   │   ├── version.go          # 版本管理
│   │   └── remote.go           # 远程操作
│   ├── storage/
│   │   ├── store.go            # 存储接口
│   │   └── sqlite.go           # SQLite 实现
│   └── models/
│       └── skill.go            # 数据模型定义
├── pkg/
│   ├── skillkit/               # Skill 开发 SDK
│   │   ├── skill.go            # Skill 接口
│   │   ├── server.go           # gRPC 服务基类
│   │   └── context.go
│   └── config/
│       └── config.go           # 配置加载
├── skills/                     # Skill 安装目录
├── data/                       # 数据目录
├── Makefile
├── README.md
├── SETUP.md
├── go.mod
└── go.sum
```

## 7. 开发指南

### 7.1 Skill 开发

#### 7.1.1 Skill 接口

```go
type Skill interface {
    Info(ctx context.Context) (*Info, error)
    Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error)
}
```

#### 7.1.2 Skill 示例

```go
package main

import (
    "context"
    "flag"
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
        BaseSkill: skillkit.NewBaseSkill("my-skill", "1.0.0", "My skill"),
    }
}

func (s *MySkill) Execute(ctx context.Context, req *skillkit.ExecuteRequest) (*skillkit.ExecuteResponse, error) {
    switch req.Method {
    case "ping":
        return &skillkit.ExecuteResponse{
            Result: []byte("pong"),
        }, nil
    default:
        return nil, fmt.Errorf("unknown method: %s", req.Method)
    }
}

func main() {
    var port int
    flag.IntVar(&port, "port", 0, "Port to listen on")
    flag.Parse()

    skill := NewMySkill()
    server := skillkit.NewServer(skill, port)

    if err := server.Start(context.Background()); err != nil {
        os.Exit(1)
    }

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    server.Stop()
}
```

### 7.2 CLI 使用

```bash
# 添加 Skill
skillctl add my-skill \
    --gitlab https://gitlab.com/example/skill.git \
    --branch main

# 列出 Skills
skillctl list

# 查看详情
skillctl get <skill-id>

# 启动/停止
skillctl start <skill-id>
skillctl stop <skill-id>

# 版本管理
skillctl versions <skill-id>
skillctl switch <skill-id> v1.0.0
skillctl pull <skill-id>

# 删除
skillctl remove <skill-id>
```

## 8. 部署与运维

### 8.1 配置文件

```yaml
# config.yaml
hub:
  grpc_addr: ":50051"
  http_addr: ":8080"

storage:
  type: sqlite
  path: /var/lib/skillhub/skillhub.db

skill:
  skills_dir: /var/lib/skillhub/skills
  port_start: 51000
  port_end: 52000
  build_timeout: 300

log:
  level: info
  format: json
```

### 8.2 系统服务

```systemd
# /etc/systemd/system/skillhub.service
[Unit]
Description=Skill Hub Daemon
After=network.target

[Service]
Type=simple
User=skillhub
ExecStart=/usr/local/bin/skillhub daemon --config /etc/skillhub/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## 9. 安全考虑

### 9.1 安全要点
- Skill 进程隔离运行
- 端口范围限制 (51000-52000)
- gRPC 服务仅监听 localhost (skill 侧)
- GitLab 访问认证支持 (可选)

### 9.2 权限模型
- Skill Hub 进程: 非 root 用户
- Skill 进程: 独立用户运行
- 仓库目录权限: 0700

## 10. 扩展点

### 10.1 未来可扩展方向
- 支持 GitHub/Gitee 等其他 Git 源
- 支持 Docker 容器化 skill
- Skill 依赖管理
- 集群部署支持
- 监控和指标采集
- Web UI 管理界面

### 10.2 插件机制
预留插件接口支持自定义：
- 存储后端扩展 (除 SQLite)
- 认证提供器
- 日志处理器
- 事件钩子

---

**文档版本**: v1.0
**最后更新**: 2026-04-28
