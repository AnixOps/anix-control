# 节点自动发现与注册 - 客户端实现指南

## 概述

本文档描述了 V2Board 节点自动发现机制的完整实现方案，包括后端 API 规范和 Go 客户端的具体实现代码。

## 架构设计

### 密钥层次结构

```
┌─────────────────────────────────────────────────────────────────┐
│  授权密钥 (AuthorizedKey)                                        │
│  - 一次性使用，用于节点首次注册                                    │
│  - 管理员在后台生成，部署时配置到节点                               │
│  - 注册成功后自动失效                                             │
└─────────────────────────────────────────────────────────────────┘
                              ↓ 注册成功后获得
┌─────────────────────────────────────────────────────────────────┐
│  节点凭证 (api_key + secret)                                     │
│  - 每个节点独立一组                                               │
│  - 用于后续所有 API 通信 (心跳、上报等)                            │
│  - 本地持久化存储                                                 │
│  - 可在管理后台撤销/重置                                          │
└─────────────────────────────────────────────────────────────────┘
```

### 工作流程

```
┌──────────────┐                         ┌──────────────┐
│   节点客户端   │                         │   V2Board    │
└──────┬───────┘                         └──────┬───────┘
       │                                        │
       │  1. 检查本地是否有凭证                   │
       │  ┌───────────────────┐                 │
       │  │ 有凭证 → 跳到步骤4   │                 │
       │  │ 无凭证 → 继续步骤2   │                 │
       │  └───────────────────┘                 │
       │                                        │
       │  2. POST /api/v1/node/register        │
       │  {auth_key, name, host, port, ...}    │
       │ ─────────────────────────────────────►│
       │                                        │
       │  3. 返回节点凭证                        │
       │  {node_id, api_key, secret}           │
       │◄───────────────────────────────────── │
       │                                        │
       │  4. 保存凭证到本地文件                   │
       │                                        │
       │  5. POST /api/v1/node/heartbeat       │
       │  Header: X-API-Key: <api_key>         │
       │  {cpu_usage, memory_usage, ...}       │
       │ ─────────────────────────────────────►│
       │                                        │
       │  6. 定时心跳 (每60秒)                   │
       │ ─────────────────────────────────────►│
       │                                        │
```

---

## 后端 API 规范

### 1. 节点注册

**Endpoint:** `POST /api/v1/node/register`

**Request:**
```json
{
    "auth_key": "abc123def456...",   // 必填: 授权密钥
    "name": "Tokyo-Node-01",         // 可选: 节点名称
    "host": "node1.example.com",     // 可选: 节点地址 (默认使用客户端IP)
    "port": 443,                     // 可选: API端口 (默认443)
    "server_version": "1.0.0",       // 可选: 节点程序版本
    "server_os": "Linux 5.15"        // 可选: 操作系统信息
}
```

**Response (Success):**
```json
{
    "message": "注册成功",
    "data": {
        "node_id": 1,
        "api_key": "a1b2c3d4e5f6...",   // 64字符
        "secret": "x9y8z7w6v5u4...",    // 64字符
        "message": "节点注册成功"
    }
}
```

**Response (Error):**
```json
{
    "message": "授权密钥无效"
}
```

### 2. 节点心跳

**Endpoint:** `POST /api/v1/node/heartbeat`

**Headers:**
```
X-API-Key: <api_key>
```

**Request:**
```json
{
    "cpu_usage": 45.5,        // CPU使用率 (%)
    "memory_usage": 60.2,     // 内存使用率 (%)
    "disk_usage": 30.0,       // 磁盘使用率 (%)
    "uptime": 86400,          // 运行时间 (秒)
    "online_users": 150,      // 在线用户数
    "upload": 1073741824,     // 本周期上传流量增量 (bytes)
    "download": 5368709120    // 本周期下载流量增量 (bytes)
}
```

**Response:**
```json
{
    "message": "ok"
}
```

---

## Go 客户端实现

### 项目结构

```
node-client/
├── main.go
├── config/
│   └── config.go
├── api/
│   └── client.go
├── credential/
│   └── store.go
├── monitor/
│   └── system.go
└── config.yaml
```

### 1. 配置文件 (config/config.go)

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// V2Board 服务器配置
	Server struct {
		URL      string `yaml:"url"`       // V2Board API地址
		Insecure bool   `yaml:"insecure"`  // 是否跳过TLS验证
	} `yaml:"server"`

	// 节点配置
	Node struct {
		AuthKey string `yaml:"auth_key"` // 授权密钥 (首次注册使用)
		Name    string `yaml:"name"`     // 节点名称
		Host    string `yaml:"host"`     // 节点地址 (可选)
		Port    int    `yaml:"port"`     // API端口
	} `yaml:"node"`

	// 心跳配置
	Heartbeat struct {
		Interval int `yaml:"interval"` // 心跳间隔 (秒)
	} `yaml:"heartbeat"`

	// 凭证存储路径
	CredentialFile string `yaml:"credential_file"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 默认值
	if cfg.Node.Port == 0 {
		cfg.Node.Port = 443
	}
	if cfg.Heartbeat.Interval == 0 {
		cfg.Heartbeat.Interval = 60
	}
	if cfg.CredentialFile == "" {
		cfg.CredentialFile = "data/credential.json"
	}

	return &cfg, nil
}
```

### 2. 凭证存储 (credential/store.go)

```go
package credential

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Credential 节点凭证
type Credential struct {
	NodeID  uint   `json:"node_id"`
	APIKey  string `json:"api_key"`
	Secret  string `json:"secret"`
}

// Store 凭证存储
type Store struct {
	filePath string
	mu       sync.RWMutex
	cred     *Credential
}

// NewStore 创建凭证存储
func NewStore(filePath string) *Store {
	return &Store{
		filePath: filePath,
	}
}

// Load 加载凭证
func (s *Store) Load() (*Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在，返回nil表示需要注册
		}
		return nil, err
	}

	var cred Credential
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, err
	}

	s.cred = &cred
	return &cred, nil
}

// Save 保存凭证
func (s *Store) Save(cred *Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}

	// 使用安全的文件权限
	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return err
	}

	s.cred = cred
	return nil
}

// Get 获取当前凭证
func (s *Store) Get() *Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cred
}

// Clear 清除凭证
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cred = nil
	return os.Remove(s.filePath)
}
```

### 3. API 客户端 (api/client.go)

```go
package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client V2Board API 客户端
type Client struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
}

// NewClient 创建 API 客户端
func NewClient(baseURL string, insecure bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure,
		},
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// SetAPIKey 设置 API Key
func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// ========== 请求/响应结构 ==========

// RegisterRequest 注册请求
type RegisterRequest struct {
	AuthKey       string `json:"auth_key"`
	Name          string `json:"name,omitempty"`
	Host          string `json:"host,omitempty"`
	Port          int    `json:"port,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
	ServerOS      string `json:"server_os,omitempty"`
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Message string `json:"message"`
	Data    struct {
		NodeID  uint   `json:"node_id"`
		APIKey  string `json:"api_key"`
		Secret  string `json:"secret"`
		Message string `json:"message"`
	} `json:"data"`
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	CPUUsage    float64 `json:"cpu_usage"`
	MemoryUsage float64 `json:"memory_usage"`
	DiskUsage   float64 `json:"disk_usage"`
	Uptime      int64   `json:"uptime"`
	OnlineUsers int     `json:"online_users"`
	Upload      int64   `json:"upload"`
	Download    int64   `json:"download"`
}

// ========== API 方法 ==========

// Register 节点注册
func (c *Client) Register(req *RegisterRequest) (*RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/node/register", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Message string `json:"message"`
		}
		json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("register failed: %s", errResp.Message)
	}

	var result RegisterResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}

// Heartbeat 发送心跳
func (c *Client) Heartbeat(req *HeartbeatRequest) error {
	if c.apiKey == "" {
		return fmt.Errorf("api key not set")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v1/node/heartbeat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: api key invalid or expired")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("heartbeat failed: %s", string(respBody))
	}

	return nil
}
```

### 4. 系统监控 (monitor/system.go)

```go
package monitor

import (
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

// SystemInfo 系统信息
type SystemInfo struct {
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   float64
	Uptime      int64
	OS          string
}

var startTime = time.Now()

// GetSystemInfo 获取系统信息
func GetSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{
		Uptime: int64(time.Since(startTime).Seconds()),
		OS:     runtime.GOOS + " " + runtime.GOARCH,
	}

	// CPU 使用率
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		info.CPUUsage = cpuPercent[0]
	}

	// 内存使用率
	memInfo, err := mem.VirtualMemory()
	if err == nil {
		info.MemoryUsage = memInfo.UsedPercent
	}

	// 磁盘使用率 (根目录)
	diskPath := "/"
	if runtime.GOOS == "windows" {
		diskPath = "C:"
	}
	diskInfo, err := disk.Usage(diskPath)
	if err == nil {
		info.DiskUsage = diskInfo.UsedPercent
	}

	return info, nil
}

// GetHostname 获取主机名
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
```

### 5. 主程序 (main.go)

```go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"node-client/api"
	"node-client/config"
	"node-client/credential"
	"node-client/monitor"
)

const Version = "1.0.0"

func main() {
	// 命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化凭证存储
	credStore := credential.NewStore(cfg.CredentialFile)

	// 初始化 API 客户端
	client := api.NewClient(cfg.Server.URL, cfg.Server.Insecure)

	// 尝试加载已有凭证
	cred, err := credStore.Load()
	if err != nil {
		log.Fatalf("加载凭证失败: %v", err)
	}

	// 如果没有凭证，需要注册
	if cred == nil {
		log.Println("未找到凭证，开始注册...")
		cred, err = register(client, cfg, credStore)
		if err != nil {
			log.Fatalf("注册失败: %v", err)
		}
		log.Printf("注册成功! NodeID: %d", cred.NodeID)
	} else {
		log.Printf("已加载凭证, NodeID: %d", cred.NodeID)
	}

	// 设置 API Key
	client.SetAPIKey(cred.APIKey)

	// 启动心跳
	go heartbeatLoop(client, cfg)

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭...")
}

// register 注册节点
func register(client *api.Client, cfg *config.Config, credStore *credential.Store) (*credential.Credential, error) {
	if cfg.Node.AuthKey == "" {
		return nil, fmt.Errorf("授权密钥未配置")
	}

	// 获取系统信息
	sysInfo, _ := monitor.GetSystemInfo()
	hostname := monitor.GetHostname()

	// 构建注册请求
	req := &api.RegisterRequest{
		AuthKey:       cfg.Node.AuthKey,
		Name:          cfg.Node.Name,
		Host:          cfg.Node.Host,
		Port:          cfg.Node.Port,
		ServerVersion: Version,
		ServerOS:      sysInfo.OS,
	}

	// 如果没有配置名称，使用主机名
	if req.Name == "" {
		req.Name = hostname
	}

	// 发送注册请求
	resp, err := client.Register(req)
	if err != nil {
		return nil, err
	}

	// 保存凭证
	cred := &credential.Credential{
		NodeID: resp.Data.NodeID,
		APIKey: resp.Data.APIKey,
		Secret: resp.Data.Secret,
	}

	if err := credStore.Save(cred); err != nil {
		return nil, fmt.Errorf("保存凭证失败: %w", err)
	}

	return cred, nil
}

// heartbeatLoop 心跳循环
func heartbeatLoop(client *api.Client, cfg *config.Config) {
	ticker := time.NewTicker(time.Duration(cfg.Heartbeat.Interval) * time.Second)
	defer ticker.Stop()

	// 立即发送一次心跳
	sendHeartbeat(client)

	for range ticker.C {
		sendHeartbeat(client)
	}
}

// sendHeartbeat 发送心跳
func sendHeartbeat(client *api.Client) {
	sysInfo, err := monitor.GetSystemInfo()
	if err != nil {
		log.Printf("获取系统信息失败: %v", err)
		return
	}

	req := &api.HeartbeatRequest{
		CPUUsage:    sysInfo.CPUUsage,
		MemoryUsage: sysInfo.MemoryUsage,
		DiskUsage:   sysInfo.DiskUsage,
		Uptime:      sysInfo.Uptime,
		OnlineUsers: 0, // TODO: 从代理服务获取
		Upload:      0, // TODO: 计算流量增量
		Download:    0,
	}

	if err := client.Heartbeat(req); err != nil {
		log.Printf("心跳失败: %v", err)
		
		// 如果是认证失败，可能需要重新注册
		if err.Error() == "unauthorized: api key invalid or expired" {
			log.Println("API Key 失效，请重新配置授权密钥并删除凭证文件")
		}
	} else {
		log.Println("心跳成功")
	}
}
```

### 6. 配置文件示例 (config.yaml)

```yaml
# V2Board 节点客户端配置

# V2Board 服务器配置
server:
  url: "https://panel.example.com"  # V2Board 面板地址
  insecure: false                   # 是否跳过 TLS 验证 (测试用)

# 节点配置
node:
  auth_key: "your-auth-key-here"    # 授权密钥 (从管理后台获取)
  name: "Tokyo-Node-01"             # 节点名称 (可选)
  host: ""                          # 节点地址 (留空则自动检测)
  port: 443                         # API 端口

# 心跳配置
heartbeat:
  interval: 60  # 心跳间隔 (秒)

# 凭证存储路径
credential_file: "data/credential.json"
```

---

## 错误处理与重试策略

### 1. 注册失败处理

```go
// 带重试的注册
func registerWithRetry(client *api.Client, cfg *config.Config, credStore *credential.Store, maxRetries int) (*credential.Credential, error) {
	var lastErr error
	
	for i := 0; i < maxRetries; i++ {
		cred, err := register(client, cfg, credStore)
		if err == nil {
			return cred, nil
		}
		
		lastErr = err
		log.Printf("注册失败 (尝试 %d/%d): %v", i+1, maxRetries, err)
		
		// 如果是授权密钥无效，不再重试
		if err.Error() == "register failed: 授权密钥无效" ||
		   err.Error() == "register failed: 授权密钥已被使用" {
			return nil, err
		}
		
		// 指数退避
		time.Sleep(time.Duration(1<<i) * time.Second)
	}
	
	return nil, lastErr
}
```

### 2. 心跳失败处理

```go
// 心跳失败计数器
var heartbeatFailCount int

func sendHeartbeatWithRecovery(client *api.Client) {
	err := sendHeartbeat(client)
	if err != nil {
		heartbeatFailCount++
		log.Printf("心跳失败 (连续 %d 次): %v", heartbeatFailCount, err)
		
		// 连续失败超过阈值，尝试恢复
		if heartbeatFailCount >= 5 {
			log.Println("心跳连续失败，尝试重新连接...")
			// 可以尝试重新注册或通知管理员
		}
	} else {
		heartbeatFailCount = 0
	}
}
```

---

## 安全建议

1. **凭证文件权限**: 凭证文件应设置为 `0600`，仅当前用户可读写
2. **TLS 验证**: 生产环境务必启用 TLS 验证
3. **授权密钥**: 授权密钥应通过安全渠道分发，使用后自动失效
4. **日志脱敏**: 日志中不应打印完整的密钥信息

---

## 依赖库

```
go get github.com/shirou/gopsutil/v3
go get gopkg.in/yaml.v3
```

---

## 部署流程

1. 管理员在 V2Board 后台生成授权密钥
2. 将授权密钥配置到节点的 `config.yaml`
3. 启动节点客户端，自动完成注册
4. 注册成功后，授权密钥自动失效
5. 凭证保存到本地，后续启动无需授权密钥

---

## 常见问题

### Q: 凭证丢失怎么办？
A: 需要管理员重新生成授权密钥，更新配置后重新注册。

### Q: 如何更换服务器？
A: 删除 `credential.json` 文件，更新配置中的授权密钥，重新启动即可。

### Q: 心跳间隔设置多少合适？
A: 建议 60 秒。过短会增加服务器负载，过长会导致状态更新不及时。
