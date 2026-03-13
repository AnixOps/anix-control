# 通用远程控制 Agent (gost-agent)

## 概述

gost-agent 是一个通用远程控制 Agent，支持 NAT 后的节点主动连接主控面板，实现类似堡垒机的能力。

## 功能特性

| 功能 | 描述 |
|------|------|
| **命令执行** | 远程执行 Shell 命令 |
| **文件管理** | 文件列表、读取、写入、删除 |
| **服务管理** | systemd/service 服务控制 |
| **端口转发** | 动态创建/删除转发规则 |
| **gost 管理** | 通过 API 管理 gost 服务 |
| **系统监控** | CPU、内存、磁盘使用率上报 |
| **WebSocket** | 实时双向通信，支持 Shell 交互 |

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      面板主控                                    │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  AgentHandler                                            │    │
│  │  - /api/v2/agent/register  注册                         │    │
│  │  - /api/v2/agent/heartbeat 心跳                         │    │
│  │  - /api/v2/agent/tasks     获取任务                     │    │
│  │  - /api/v2/agent/ws        WebSocket 实时通信           │    │
│  │  - /admin/agent/execute    执行命令                     │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
                              ↑
                   Agent 主动连接 (出站流量)
                              │
┌─────────────────────────────────────────────────────────────────┐
│                   转发节点 (NAT 后)                              │
│  ┌─────────────────┐    ┌─────────────────┐                     │
│  │  gost-agent     │───→│  gost (本地)     │                    │
│  │  主动连接主控    │    │  API: 127.0.0.1 │                    │
│  └─────────────────┘    └─────────────────┘                     │
└─────────────────────────────────────────────────────────────────┘
```

## 部署方式

### 方式一：命令行参数

```bash
./gost-agent \
  -panel https://panel.example.com \
  -token your-node-token \
  -id 1 \
  -interval 30
```

### 方式二：配置文件

```json
// agent.json
{
  "panel_url": "https://panel.example.com",
  "panel_token": "your-node-token",
  "node_id": 1,
  "poll_interval": 30,

  "enable_shell": true,
  "enable_file_manage": true,
  "enable_service": true,
  "enable_port_forward": true,
  "enable_gost": true,
  "enable_monitor": true,

  "gost_api_url": "http://127.0.0.1:18080",
  "gost_token": "gost-api-token",

  "allowed_commands": ["*"],
  "allowed_paths": ["/"]
}
```

```bash
./gost-agent -config agent.json
```

### 方式三：环境变量

```bash
export AGENT_PANEL_URL=https://panel.example.com
export AGENT_TOKEN=your-node-token
export AGENT_NODE_ID=1

./gost-agent
```

### 方式四：Systemd 服务

```ini
# /etc/systemd/system/gost-agent.service
[Unit]
Description=gost-agent remote control
After=network.target

[Service]
Type=simple
ExecStart=/opt/gost-agent/gost-agent -config /opt/gost-agent/config.json
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
```

```bash
systemctl enable gost-agent
systemctl start gost-agent
```

## API 端点

### Agent 端点（公开）

| 端点 | 方法 | 描述 |
|------|------|------|
| `/api/v2/agent/register` | POST | 注册到面板 |
| `/api/v2/agent/heartbeat` | POST | 发送心跳 |
| `/api/v2/agent/tasks` | GET | 获取待执行任务 |
| `/api/v2/agent/result` | POST | 上报任务结果 |
| `/api/v2/agent/monitor` | POST | 上报监控数据 |
| `/api/v2/agent/ws` | GET | WebSocket 连接 |

### 管理端点（需认证）

| 端点 | 方法 | 描述 |
|------|------|------|
| `/admin/agent/list` | GET | 获取在线 Agent 列表 |
| `/admin/agent/tasks` | POST | 向节点下发任务 |
| `/admin/agent/execute` | POST | 在节点上执行命令 |

## 任务类型

### 命令执行

```json
{
  "node_id": 1,
  "type": "command",
  "action": "ls",
  "params": {
    "args": ["-la", "/tmp"],
    "env": {"PATH": "/usr/bin"}
  },
  "timeout": 30
}
```

### 文件操作

```json
{
  "node_id": 1,
  "type": "file",
  "action": "list",
  "params": {"path": "/var/log"}
}
```

支持的文件操作：
- `list` - 列出目录内容
- `read` - 读取文件内容
- `write` - 写入文件
- `delete` - 删除文件/目录
- `mkdir` - 创建目录
- `stat` - 获取文件信息

### 服务管理

```json
{
  "node_id": 1,
  "type": "service",
  "action": "restart",
  "params": {"name": "nginx"}
}
```

支持的服务操作：
- `list` - 列出所有服务
- `status` - 获取服务状态
- `start` - 启动服务
- `stop` - 停止服务
- `restart` - 重启服务
- `enable` - 启用服务开机启动
- `disable` - 禁用服务开机启动

### gost 管理

```json
{
  "node_id": 1,
  "type": "gost",
  "action": "create_service",
  "params": {
    "service": {
      "name": "forward-10000",
      "addr": ":10000",
      "handler": {"type": "tcp"},
      "listener": {"type": "tcp"},
      "forwarder": {
        "nodes": [{"name": "target", "addr": "1.2.3.4:80"}]
      }
    }
  }
}
```

支持的 gost 操作：
- `services` - 获取服务列表
- `create_service` - 创建服务
- `delete_service` - 删除服务
- `stats` - 获取统计信息

## 安全配置

### 命令白名单

```json
{
  "allowed_commands": ["ls", "cat", "grep", "systemctl"]
}
```

或允许所有：
```json
{
  "allowed_commands": ["*"]
}
```

### 路径限制

```json
{
  "allowed_paths": ["/var/log", "/etc/nginx", "/home"]
}
```

## WebSocket Shell

通过 WebSocket 实现交互式 Shell：

```javascript
// 前端示例
const ws = new WebSocket('wss://panel.example.com/api/v2/agent/ws');

// 认证
ws.send(JSON.stringify({
  type: 'auth',
  node_id: 1,
  token: 'your-token'
}));

// 执行命令
ws.send(JSON.stringify({
  type: 'shell',
  session_id: 'session-1',
  command: 'ls -la'
}));

// 接收输出
ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === 'shell_output') {
    console.log(msg.output);
  }
};
```

## 与 gost 转发结合

1. 在 NAT 后节点上启动 gost 和 gost-agent
2. gost-agent 主动连接面板
3. 面板通过 gost-agent 下发转发规则
4. gost-agent 调用本地 gost API 应用规则

```bash
# 节点上运行
./gost -C gost.yml &
./gost-agent -panel https://panel.example.com -token xxx -id 1
```

面板上创建转发规则后，规则会自动同步到节点的 gost。