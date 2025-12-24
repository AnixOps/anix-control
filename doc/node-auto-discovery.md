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
       │  2. POST /api/v2/node/register        │
       │  {auth_key, name, host, port, ...}    │
       │ ─────────────────────────────────────►│
       │                                        │
       │  3. 返回节点凭证                        │
       │  {node_id, api_key, secret}           │
       │◄───────────────────────────────────── │
       │                                        │
       │  4. 保存凭证到本地文件                   │
       │                                        │
       │  5. POST /api/v2/node/heartbeat       │
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

**Endpoint:** `POST /api/v2/node/register`

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

**Endpoint:** `POST /api/v2/node/heartbeat`

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

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v2/node/register", bytes.NewReader(body))
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

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v2/node/heartbeat", bytes.NewReader(body))
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
5. **请求签名**: 建议启用 HMAC 签名验证，防止中间人攻击

---

## 安全增强 - 请求签名

服务端已支持请求签名验证，客户端只需在请求中添加签名 Header 即可启用此安全特性。

### 签名算法

```
签名 = HMAC-SHA256(timestamp + method + path + body, secret)
```

### 客户端签名实现

在 `api/client.go` 中添加签名功能：

```go
package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client V2Board API 客户端
type Client struct {
	baseURL       string
	httpClient    *http.Client
	apiKey        string
	secret        string  // 用于签名
	enableSign    bool    // 是否启用签名
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
		enableSign: true, // 默认启用签名
	}
}

// SetCredentials 设置 API Key 和 Secret
func (c *Client) SetCredentials(apiKey, secret string) {
	c.apiKey = apiKey
	c.secret = secret
}

// SetEnableSign 设置是否启用签名
func (c *Client) SetEnableSign(enable bool) {
	c.enableSign = enable
}

// signRequest 为请求添加签名
func (c *Client) signRequest(req *http.Request, body []byte) {
	if !c.enableSign || c.secret == "" {
		return
	}

	// 时间戳
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// 生成随机 nonce (防重放)
	nonceBytes := make([]byte, 16)
	rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	// 构建签名字符串: timestamp + method + path + body
	signData := timestamp + req.Method + req.URL.Path + string(body)

	// 计算 HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(c.secret))
	mac.Write([]byte(signData))
	signature := hex.EncodeToString(mac.Sum(nil))

	// 设置签名相关 Header
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", signature)
}

// Heartbeat 发送心跳 (带签名)
func (c *Client) Heartbeat(req *HeartbeatRequest) error {
	if c.apiKey == "" {
		return fmt.Errorf("api key not set")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/api/v2/node/heartbeat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)

	// 添加签名
	c.signRequest(httpReq, body)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: api key invalid or expired")
	}

	if resp.StatusCode == http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(respBody, &errResp)
		
		// 签名相关错误
		switch errResp.Code {
		case "INVALID_SIGNATURE":
			return fmt.Errorf("签名验证失败，请检查 secret 是否正确")
		case "REQUEST_EXPIRED":
			return fmt.Errorf("请求已过期，请检查系统时间是否同步")
		case "DUPLICATE_REQUEST":
			return fmt.Errorf("重复请求")
		}
		return fmt.Errorf("heartbeat failed: %s", errResp.Message)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("heartbeat failed: %s", string(respBody))
	}

	return nil
}
```

### 凭证更新

注册成功后需要同时保存 `api_key` 和 `secret`：

```go
// Credential 节点凭证
type Credential struct {
	NodeID  uint   `json:"node_id"`
	APIKey  string `json:"api_key"`
	Secret  string `json:"secret"`  // 添加 secret 字段
}

// 注册成功后保存
cred := &credential.Credential{
	NodeID: resp.Data.NodeID,
	APIKey: resp.Data.APIKey,
	Secret: resp.Data.Secret,  // 保存 secret
}

// 设置客户端凭证
client.SetCredentials(cred.APIKey, cred.Secret)
```

### 签名验证流程

```
客户端                                服务端
   │                                    │
   │ 1. 生成时间戳 + nonce               │
   │                                    │
   │ 2. 计算签名                         │
   │    sig = HMAC(ts+method+path+body) │
   │                                    │
   │ 3. 发送请求                         │
   │    Header: X-API-Key               │
   │    Header: X-Timestamp             │
   │    Header: X-Nonce                 │
   │    Header: X-Signature             │
   │ ──────────────────────────────────►│
   │                                    │
   │                    4. 验证时间戳 (±5分钟)
   │                    5. 检查 nonce 防重放
   │                    6. 重新计算签名并比对
   │                                    │
   │◄────────────────────────────────── │
   │              成功/失败              │
```

### 签名验证的安全特性

| 特性 | 说明 |
|------|-----|
| **防篡改** | 修改请求体会导致签名不匹配 |
| **防重放** | nonce 一次性使用，5分钟内有效 |
| **时效性** | 时间戳超过5分钟的请求被拒绝 |
| **身份绑定** | secret 与节点绑定，泄露不影响其他节点 |

---

## 安全增强 - 凭证加密存储

### 使用机器特征加密

```go
package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/denisbrodbeck/machineid"
)

// EncryptedStore 加密凭证存储
type EncryptedStore struct {
	filePath string
	key      []byte
}

// NewEncryptedStore 创建加密存储
func NewEncryptedStore(filePath string) (*EncryptedStore, error) {
	key, err := deriveKey()
	if err != nil {
		return nil, err
	}
	return &EncryptedStore{
		filePath: filePath,
		key:      key,
	}, nil
}

// deriveKey 从机器特征派生加密密钥
func deriveKey() ([]byte, error) {
	// 获取机器唯一 ID
	id, err := machineid.ID()
	if err != nil {
		// 降级：使用主机名
		hostname, _ := os.Hostname()
		id = hostname
	}

	// 加盐并哈希
	salt := "v2board-node-credential-v1"
	h := sha256.Sum256([]byte(id + salt))
	return h[:], nil
}

// Save 加密保存凭证
func (s *EncryptedStore) Save(cred *Credential) error {
	// JSON 序列化
	plaintext, err := json.Marshal(cred)
	if err != nil {
		return err
	}

	// AES-GCM 加密
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	// 确保目录存在
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// 写入文件
	return os.WriteFile(s.filePath, ciphertext, 0600)
}

// Load 解密加载凭证
func (s *EncryptedStore) Load() (*Credential, error) {
	ciphertext, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// AES-GCM 解密
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed: %w", err)
	}

	var cred Credential
	if err := json.Unmarshal(plaintext, &cred); err != nil {
		return nil, err
	}

	return &cred, nil
}
```

### 依赖

```
go get github.com/denisbrodbeck/machineid
```

---

## 安全增强 - 日志脱敏

客户端也应该对日志进行脱敏处理：

```go
package utils

// Redact 脱敏字符串
func Redact(s string) string {
	if s == "" {
		return ""
	}
	length := len(s)
	if length <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[length-4:]
}

// 使用示例
log.Printf("使用凭证: NodeID=%d, APIKey=%s", cred.NodeID, Redact(cred.APIKey))
// 输出: 使用凭证: NodeID=1, APIKey=a1b2****e5f6
```

---

## 依赖库

```
go get github.com/shirou/gopsutil/v3
go get gopkg.in/yaml.v3
go get github.com/denisbrodbeck/machineid
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
