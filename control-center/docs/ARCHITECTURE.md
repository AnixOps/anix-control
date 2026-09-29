# AnixOps Control Center - 系统架构设计

## 一、整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        AnixOps Control Center                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │  Flutter    │  │   Web App   │  │      CLI / TUI          │ │
│  │  Mobile/Desktop │  (Vue.js)  │  │   (Go)                  │ │
│  └──────┬──────┘  └──────┬──────┘  └───────────┬─────────────┘ │
│         │                │                      │               │
│         └────────────────┼──────────────────────┘               │
│                          │                                      │
│                          ▼                                      │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    API Gateway (Cloudflare Workers)         ││
│  │  - JWT Authentication                                       ││
│  │  - Rate Limiting                                            ││
│  │  - WebSocket Support                                        ││
│  └─────────────────────────────┬───────────────────────────────┘│
│                                │                                │
│         ┌──────────────────────┼──────────────────────┐         │
│         │                      │                      │         │
│         ▼                      ▼                      ▼         │
│  ┌─────────────┐      ┌─────────────────┐     ┌──────────────┐ │
│  │  D1 Database │      │  KV Namespace   │     │  R2 Storage  │ │
│  │  - Users    │      │  - Sessions     │     │  - Playbooks │ │
│  │  - Nodes    │      │  - Cache        │     │  - Logs      │ │
│  │  - Logs     │      │  - Credentials  │     │  - Backups   │ │
│  └─────────────┘      └─────────────────┘     └──────────────┘ │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    Agent Service (on each node)             ││
│  │  - Ansible Runner                                           ││
│  │  - System Metrics Collection                                ││
│  │  - Health Check                                             ││
│  │  - Plugin Runtime                                           ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 二、核心组件

### 1. Ansible 编排引擎

```
┌─────────────────────────────────────────────────────────────┐
│                    Ansible Orchestration                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Playbook Manager                    │   │
│  │  - Built-in Playbooks (预设任务)                     │   │
│  │  - GitHub Sync (从 GitHub 同步)                      │   │
│  │  - Custom Upload (自定义上传)                        │   │
│  │  - Version Control (版本管理)                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Inventory Manager                   │   │
│  │  - Node Groups (节点分组)                            │   │
│  │  - Dynamic Inventory (动态清单)                      │   │
│  │  - Variables (变量管理)                              │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Execution Engine                    │   │
│  │  - Task Queue (任务队列)                             │   │
│  │  - Parallel Execution (并行执行)                     │   │
│  │  - Rollback Support (回滚支持)                       │   │
│  │  - Real-time Logs (实时日志)                         │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │                  Template Engine                     │   │
│  │  - Jinja2 Templates (模板)                           │   │
│  │  - Variable Substitution (变量替换)                  │   │
│  │  - Secret Management (密钥管理)                      │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 2. Plugin 系统

```
┌─────────────────────────────────────────────────────────────┐
│                      Plugin System                           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │                    Core Plugins                       │ │
│  ├───────────────────────────────────────────────────────┤ │
│  │  • WebSSH     - Browser-based SSH terminal           │ │
│  │  • Monitor    - Node monitoring & metrics            │ │
│  │  • Backup     - Automated backup & restore           │ │
│  │  • Alert      - Notification & alerting              │ │
│  │  • Logs       - Log aggregation & search             │ │
│  │  • Scheduler  - Task scheduling                      │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │                  Server Plugins                       │ │
│  ├───────────────────────────────────────────────────────┤ │
│  │  • V2Ray/XRay - Proxy server management              │ │
│  │  • Trojan     - Trojan server management             │ │
│  │  • Shadowsocks- SS server management                 │ │
│  │  • Hysteria   - Hysteria server management           │ │
│  │  • WireGuard  - VPN server management                │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │                  Plugin Interface                     │ │
│  ├───────────────────────────────────────────────────────┤ │
│  │  interface Plugin {                                   │ │
│  │    name: string                                       │ │
│  │    version: string                                    │ │
│  │    init(config): Promise<void>                       │ │
│  │    execute(params): Promise<Result>                  │ │
│  │    validate(params): Promise<boolean>                │ │
│  │    cleanup(): Promise<void>                          │ │
│  │  }                                                    │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 3. Playbook 市场

```
┌─────────────────────────────────────────────────────────────┐
│                    Playbook Marketplace                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │               Built-in Playbooks (预设)              │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │                                                     │   │
│  │  📦 System Security                                 │   │
│  │  ├── install-fail2ban    - 安装 Fail2ban           │   │
│  │  ├── configure-firewall  - 配置防火墙              │   │
│  │  ├── harden-ssh         - SSH 加固                 │   │
│  │  ├── install-clamav     - 安装杀毒软件             │   │
│  │  └── system-hardening   - 系统全面加固             │   │
│  │                                                     │   │
│  │  📦 Infrastructure                                   │   │
│  │  ├── install-docker     - 安装 Docker              │   │
│  │  ├── install-nginx      - 安装 Nginx               │   │
│  │  ├── install-nodejs     - 安装 Node.js             │   │
│  │  ├── install-mysql      - 安装 MySQL               │   │
│  │  └── install-redis      - 安装 Redis               │   │
│  │                                                     │   │
│  │  📦 SSL & Certificates                              │   │
│  │  ├── install-certbot    - 安装 Certbot             │   │
│  │  ├── issue-letsencrypt  - 申请 Let's Encrypt 证书  │   │
│  │  └── auto-renew-certs   - 自动续期证书             │   │
│  │                                                     │   │
│  │  📦 Proxy Servers                                   │   │
│  │  ├── deploy-xray       - 部署 XRay                 │   │
│  │  ├── deploy-trojan     - 部署 Trojan               │   │
│  │  ├── deploy-hysteria   - 部署 Hysteria             │   │
│  │  └── deploy-wireguard  - 部署 WireGuard            │   │
│  │                                                     │   │
│  │  📦 Maintenance                                     │   │
│  │  ├── system-update     - 系统更新                  │   │
│  │  ├── cleanup-disk      - 清理磁盘                  │   │
│  │  ├── backup-config     - 备份配置                  │   │
│  │  └── rotate-logs       - 日志轮转                  │   │
│  │                                                     │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │               GitHub Sync (同步仓库)                 │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  - Official Repo: github.com/anixops/playbooks      │   │
│  │  - Community Repo: github.com/anixops/community     │   │
│  │  - Custom Repo: User-defined repositories          │   │
│  │  - Auto-sync: Schedule-based synchronization       │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │               Custom Upload (自定义上传)             │   │
│  ├─────────────────────────────────────────────────────┤   │
│  │  - Upload YAML/ZIP files                           │   │
│  │  - Validation & Security Scan                      │   │
│  │  - Store in R2                                     │   │
│  │  - Share with team                                 │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 三、数据模型

### 数据库表结构 (D1)

```sql
-- 用户表
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT,
    role TEXT DEFAULT 'viewer',  -- admin, operator, viewer
    enabled INTEGER DEFAULT 1,
    auth_provider TEXT DEFAULT 'local',
    last_login_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 节点表
CREATE TABLE nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    host TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    status TEXT DEFAULT 'offline',  -- online, offline, maintenance
    group_id INTEGER,
    tags TEXT,  -- JSON array
    config TEXT,  -- JSON object
    last_seen DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 节点组表
CREATE TABLE node_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    parent_id INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Playbook 表
CREATE TABLE playbooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    category TEXT,  -- security, infrastructure, proxy, maintenance
    source TEXT,  -- built-in, github, custom
    github_repo TEXT,
    github_path TEXT,
    version TEXT,
    storage_key TEXT,  -- R2 key
    variables TEXT,  -- JSON schema for variables
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 执行任务表
CREATE TABLE tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playbook_id INTEGER NOT NULL,
    status TEXT DEFAULT 'pending',  -- pending, running, success, failed
    trigger_type TEXT,  -- manual, scheduled, webhook
    triggered_by INTEGER,  -- user_id
    target_nodes TEXT,  -- JSON array of node IDs
    variables TEXT,  -- JSON object
    result TEXT,  -- JSON object with results per node
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 任务日志表
CREATE TABLE task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    node_id INTEGER,
    level TEXT,  -- info, warning, error
    message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 调度任务表
CREATE TABLE schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    playbook_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    cron TEXT NOT NULL,
    target_nodes TEXT,
    variables TEXT,
    enabled INTEGER DEFAULT 1,
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 插件表
CREATE TABLE plugins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    display_name TEXT,
    version TEXT,
    description TEXT,
    author TEXT,
    enabled INTEGER DEFAULT 1,
    config TEXT,  -- JSON configuration
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- API Tokens 表
CREATE TABLE api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    token TEXT NOT NULL,
    last_used DATETIME,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 审计日志表
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL,
    resource TEXT,
    ip TEXT,
    user_agent TEXT,
    details TEXT,  -- JSON
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## 四、API 端点规划

### 认证 API
```
POST   /api/v1/auth/login          - 登录
POST   /api/v1/auth/register        - 注册
POST   /api/v1/auth/logout          - 登出
POST   /api/v1/auth/refresh         - 刷新Token
PUT    /api/v1/auth/password        - 修改密码
```

### 节点管理 API
```
GET    /api/v1/nodes               - 节点列表
POST   /api/v1/nodes               - 创建节点
GET    /api/v1/nodes/:id           - 节点详情
PUT    /api/v1/nodes/:id           - 更新节点
DELETE /api/v1/nodes/:id           - 删除节点
POST   /api/v1/nodes/:id/start     - 启动节点
POST   /api/v1/nodes/:id/stop      - 停止节点
POST   /api/v1/nodes/:id/restart   - 重启节点
GET    /api/v1/nodes/:id/stats     - 节点统计
GET    /api/v1/nodes/:id/logs      - 节点日志
POST   /api/v1/nodes/:id/test      - 测试连接

GET    /api/v1/node-groups         - 节点组列表
POST   /api/v1/node-groups         - 创建节点组
PUT    /api/v1/node-groups/:id     - 更新节点组
DELETE /api/v1/node-groups/:id     - 删除节点组
```

### Playbook API
```
GET    /api/v1/playbooks           - Playbook列表
POST   /api/v1/playbooks           - 上传Playbook
GET    /api/v1/playbooks/:id       - Playbook详情
PUT    /api/v1/playbooks/:id       - 更新Playbook
DELETE /api/v1/playbooks/:id       - 删除Playbook
POST   /api/v1/playbooks/:id/run   - 执行Playbook

GET    /api/v1/playbooks/built-in  - 预设Playbook列表
POST   /api/v1/playbooks/sync      - 从GitHub同步
GET    /api/v1/playbooks/categories - Playbook分类
```

### 任务 API
```
GET    /api/v1/tasks               - 任务列表
GET    /api/v1/tasks/:id           - 任务详情
GET    /api/v1/tasks/:id/logs      - 任务日志
POST   /api/v1/tasks/:id/cancel    - 取消任务
POST   /api/v1/tasks/:id/retry     - 重试任务
```

### 调度 API
```
GET    /api/v1/schedules           - 调度列表
POST   /api/v1/schedules           - 创建调度
PUT    /api/v1/schedules/:id       - 更新调度
DELETE /api/v1/schedules/:id       - 删除调度
POST   /api/v1/schedules/:id/toggle - 启用/禁用
```

### 插件 API
```
GET    /api/v1/plugins             - 插件列表
GET    /api/v1/plugins/:name       - 插件详情
POST   /api/v1/plugins/:name/execute - 执行插件
PUT    /api/v1/plugins/:name/config - 配置插件
POST   /api/v1/plugins/:name/enable - 启用插件
POST   /api/v1/plugins/:name/disable - 禁用插件
```

### WebSSH API
```
GET    /api/v1/ssh/connect/:nodeId - WebSocket连接
```

## 五、预设 Playbook 详情

### 1. 系统安全类

#### install-fail2ban
```yaml
---
- name: Install and configure Fail2ban
  hosts: all
  become: yes
  vars:
    fail2ban_bantime: 3600
    fail2ban_findtime: 600
    fail2ban_maxretry: 5
  tasks:
    - name: Install Fail2ban
      apt:
        name: fail2ban
        state: present
        update_cache: yes

    - name: Configure Fail2ban
      template:
        src: jail.local.j2
        dest: /etc/fail2ban/jail.local
      notify: Restart Fail2ban

  handlers:
    - name: Restart Fail2ban
      service:
        name: fail2ban
        state: restarted
```

#### configure-firewall
```yaml
---
- name: Configure UFW Firewall
  hosts: all
  become: yes
  vars:
    allowed_ports:
      - 22    # SSH
      - 80    # HTTP
      - 443   # HTTPS
    deny_incoming: true
  tasks:
    - name: Install UFW
      apt:
        name: ufw
        state: present

    - name: Allow specified ports
      ufw:
        rule: allow
        port: "{{ item }}"
      loop: "{{ allowed_ports }}"

    - name: Enable UFW
      ufw:
        state: enabled
        policy: deny
        direction: incoming
```

#### harden-ssh
```yaml
---
- name: Harden SSH Configuration
  hosts: all
  become: yes
  vars:
    ssh_port: 22
    permit_root_login: no
    password_authentication: no
    max_auth_tries: 3
  tasks:
    - name: Configure SSH
      lineinfile:
        path: /etc/ssh/sshd_config
        regexp: "{{ item.regexp }}"
        line: "{{ item.line }}"
      loop:
        - { regexp: '^#?Port', line: 'Port {{ ssh_port }}' }
        - { regexp: '^#?PermitRootLogin', line: 'PermitRootLogin {{ permit_root_login }}' }
        - { regexp: '^#?PasswordAuthentication', line: 'PasswordAuthentication {{ password_authentication }}' }
        - { regexp: '^#?MaxAuthTries', line: 'MaxAuthTries {{ max_auth_tries }}' }
      notify: Restart SSH

  handlers:
    - name: Restart SSH
      service:
        name: sshd
        state: restarted
```

### 2. 基础设施类

#### install-docker
```yaml
---
- name: Install Docker
  hosts: all
  become: yes
  vars:
    docker_compose_version: "2.21.0"
  tasks:
    - name: Install dependencies
      apt:
        name:
          - apt-transport-https
          - ca-certificates
          - curl
          - gnupg
          - lsb-release
        state: present
        update_cache: yes

    - name: Add Docker GPG key
      apt_key:
        url: https://download.docker.com/linux/ubuntu/gpg
        state: present

    - name: Add Docker repository
      apt_repository:
        repo: "deb https://download.docker.com/linux/ubuntu {{ ansible_lsb.codename }} stable"
        state: present

    - name: Install Docker
      apt:
        name:
          - docker-ce
          - docker-ce-cli
          - containerd.io
        state: present
        update_cache: yes

    - name: Install Docker Compose
      get_url:
        url: "https://github.com/docker/compose/releases/download/v{{ docker_compose_version }}/docker-compose-linux-x86_64"
        dest: /usr/local/bin/docker-compose
        mode: '0755'
```

### 3. 代理服务器类

#### deploy-xray
```yaml
---
- name: Deploy XRay Server
  hosts: all
  become: yes
  vars:
    xray_version: "1.8.6"
    xray_port: 443
    xray_uuid: "{{ lookup('password', '/dev/null length=36 chars=ascii_letters,digits') }}"
  tasks:
    - name: Download XRay
      get_url:
        url: "https://github.com/XTLS/Xray-core/releases/download/v{{ xray_version }}/Xray-linux-64.zip"
        dest: /tmp/xray.zip

    - name: Extract XRay
      unarchive:
        src: /tmp/xray.zip
        dest: /usr/local/bin
        remote_src: yes

    - name: Create XRay config directory
      file:
        path: /etc/xray
        state: directory

    - name: Configure XRay
      template:
        src: config.json.j2
        dest: /etc/xray/config.json
      notify: Restart XRay

    - name: Create XRay systemd service
      template:
        src: xray.service.j2
        dest: /etc/systemd/system/xray.service
      notify: Restart XRay

  handlers:
    - name: Restart XRay
      systemd:
        name: xray
        state: restarted
        daemon_reload: yes
        enabled: yes
```

## 六、实现优先级

### Phase 1: 核心功能 (Week 1-2)
1. ✅ 用户认证系统
2. ✅ 节点管理基础
3. ✅ Workers API 部署
4. 🔄 Playbook 基础结构
5. 🔄 Ansible 执行引擎

### Phase 2: 编排系统 (Week 3-4)
1. 预设 Playbook 实现
2. Playbook 上传功能
3. 任务执行和日志
4. GitHub 同步功能

### Phase 3: 插件系统 (Week 5-6)
1. WebSSH 插件
2. 监控插件
3. 插件市场

### Phase 4: 高级功能 (Week 7-8)
1. 调度系统
2. 告警通知
3. 备份恢复
4. 多租户支持

## 七、技术栈

### 后端
- **Cloudflare Workers** - API Gateway
- **Cloudflare D1** - SQLite 数据库
- **Cloudflare KV** - 缓存和会话
- **Cloudflare R2** - 文件存储
- **Ansible** - 编排引擎
- **Go** - Agent 服务

### 前端
- **Flutter** - 跨平台客户端
- **Vue.js** - Web 应用
- **Go (Bubble Tea)** - TUI

### 通信
- **WebSocket** - 实时日志和 SSH
- **REST API** - 标准接口
- **gRPC** - Agent 通信

---

*此文档为架构设计，将逐步实现所有功能。*