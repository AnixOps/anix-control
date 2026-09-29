# AnixOps Control Center

A unified TUI/GUI control center for managing all AnixOps products.

This project lives in `control-center/` inside [`AnixOps/anix-control`](https://github.com/AnixOps/anix-control).
It was imported from the now-archived `AnixOps/Anixops-control-center` repository; the Cloudflare Workers API
(formerly `AnixOps/Anixops-control-center-worker`) lives in [`workers/`](workers/).

## Features

- **Plugin Architecture**: Modular plugin system for extensibility
- **Multiple Interfaces**: TUI, Web GUI, CLI, REST API
- **Enterprise Security**: JWT, OAuth, LDAP, SAML authentication with RBAC
- **Product Integration**:
  - Ansible automation engine
  - v2board panel management
  - V2bX node management
  - AnixOps-agent remote control

## Quick Start

```bash
# Build
go build -o anixops cmd/anixops/main.go

# Run TUI
./anixops tui

# Start API server
./anixops server -c config.yaml

# Run Ansible playbook
./anixops ansible run deploy.yml -i inventory/hosts

# List nodes
./anixops nodes list
```

### Kernel Plugin Lifecycle

The Web GUI exposes the Control kernel lifecycle at `/plugins`. Workers
authentication and legacy pages keep using `VITE_API_URL` and `/api/v1`. The
plugin page connects separately to Control through `/api/v2/login`, then calls
its authenticated `/api/v3` API. The Control token stays in browser session
storage; a Control 401 disconnects only that session. MFA code challenges are
handled in the plugin page. An MFA enrollment requirement blocks connection
until the account is enrolled in Anix Control. This page manages the plugin
lifecycle; signed plugin WebUI modules are rendered by Control's own verified
same-origin frontend.

Set `VITE_KERNEL_API_URL` to the Control API origin or `/api/v3` URL in
production. It defaults to same-origin `/api/v3`, which the Vite development
server proxies to `localhost:8080`. Production hosting must route both
`/api/v2/login` and `/api/v3` to the same Control instance. Keep verified
WebUI assets on that same origin.

```bash
cd web
npm install
VITE_API_URL=https://api.anixops.com/api/v1 VITE_KERNEL_API_URL=/api/v3 npm run dev
```

Run the real local Control browser gate separately from the mocked suite:

```bash
ANIXOPS_REAL_CONTROL_WEB_PORT=3013 \
ANIXOPS_REAL_CONTROL_API_PORT=38083 \
npm run test:e2e:real-control -- --reporter=line
```

This gate starts an isolated `anix-control` process, performs `/api/v2/login`,
and reads `/api/v3/plugins` plus `/api/v3/plugin-installations`. Workers auth
is synthetic local storage in this test. By default the gate is read-only after
bootstrap; set `ANIXOPS_AGENT_ROOT` and `ANIXOPS_REAL_CONTROL_LIFECYCLE=1` to
build a signed `machine-telemetry` package and exercise Control-target
disable/enable through the Center page. That remains isolated local-process
evidence; live Agent mutation still requires staging evidence.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ANIXOPS CONTROL CENTER                               │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                      │
│  │ TUI Interface │  │  Web GUI     │  │ CLI Interface│                      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘                      │
│         └─────────────────┼─────────────────┘                               │
│                           ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                        Core API Layer (Go)                            │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                           ▼                                                 │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                      Plugin Manager (Core)                            │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                           ▼                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐       │
│  │ Ansible     │  │ v2board     │  │ V2bX        │  │ Agent       │       │
│  │ Plugin      │  │ Plugin      │  │ Plugin      │  │ Plugin      │       │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘       │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Plugin Development

```go
package myplugin

import (
    "context"
    "github.com/AnixOps/anix-control/control-center/internal/core/plugin"
)

type MyPlugin struct{}

func (p *MyPlugin) Info() plugin.PluginInfo {
    return plugin.PluginInfo{
        Name:        "myplugin",
        Version:     "1.0.0",
        Description: "My custom plugin",
    }
}

func (p *MyPlugin) Init(ctx context.Context, config map[string]interface{}) error {
    return nil
}

func (p *MyPlugin) Start(ctx context.Context) error {
    return nil
}

func (p *MyPlugin) Stop(ctx context.Context) error {
    return nil
}

func (p *MyPlugin) HealthCheck(ctx context.Context) error {
    return nil
}

func (p *MyPlugin) Capabilities() []string {
    return []string{"do_something"}
}

// Optional: Implement ExecutablePlugin for action execution
func (p *MyPlugin) Execute(ctx context.Context, action string, params map[string]interface{}) (plugin.Result, error) {
    return plugin.Result{Success: true}, nil
}
```

## API Endpoints

### Auth
- `POST /api/v1/auth/login` - Login
- `POST /api/v1/auth/refresh` - Refresh token

### Plugins
- `GET /api/v1/plugins` - List plugins
- `GET /api/v1/plugins/:name` - Get plugin info
- `POST /api/v1/plugins/:name/execute` - Execute plugin action

### Nodes
- `GET /api/v1/nodes` - List nodes
- `POST /api/v1/nodes` - Create node
- `DELETE /api/v1/nodes/:id` - Delete node

### Playbooks
- `GET /api/v1/playbooks` - List playbooks
- `POST /api/v1/playbooks/run` - Run playbook

### Users
- `GET /api/v1/users` - List users
- `POST /api/v1/users/:id/ban` - Ban user
- `POST /api/v1/users/:id/unban` - Unban user

## Configuration

See `configs/config.yaml` for configuration options.

## License

MIT
