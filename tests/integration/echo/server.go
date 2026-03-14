package echo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EchoServer 内置 Echo 服务器，用于测试代理连通性
// 不依赖外部网络，完全本地化测试
type EchoServer struct {
	port    int
	server  *http.Server
	ln      net.Listener
	running bool
	mu      sync.Mutex

	// 统计
	requests int64
	bytesIn  int64
	bytesOut int64
}

// NewEchoServer 创建 Echo 服务器
func NewEchoServer(port int) *EchoServer {
	return &EchoServer{
		port: port,
	}
}

// Start 启动服务器
func (s *EchoServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// 创建监听器
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return err
	}
	s.ln = ln

	// 获取实际端口
	s.port = ln.Addr().(*net.TCPAddr).Port

	// 创建 HTTP 服务器
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleEcho)
	mux.HandleFunc("/echo", s.handleEcho)
	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/generate_204", s.handleGenerate204)

	s.server = &http.Server{
		Handler: mux,
	}

	s.running = true

	// 启动服务器
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			// 服务器错误
		}
	}()

	return nil
}

// Stop 停止服务器
func (s *EchoServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// Port 返回端口
func (s *EchoServer) Port() int {
	return s.port
}

// URL 返回服务器 URL
func (s *EchoServer) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// Stats 返回统计信息
func (s *EchoServer) Stats() (requests, bytesIn, bytesOut int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests, s.bytesIn, s.bytesOut
}

// IsRunning 检查是否运行中
func (s *EchoServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// 处理函数
func (s *EchoServer) handleEcho(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r.Body.Close()

	s.mu.Lock()
	s.bytesIn += int64(len(body))
	s.mu.Unlock()

	// 设置响应头
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-Echo-Server", "v1")
	w.Header().Set("X-Request-Time", time.Now().Format(time.RFC3339))

	// 返回请求信息
	response := fmt.Sprintf("Echo Response\n")
	response += fmt.Sprintf("Method: %s\n", r.Method)
	response += fmt.Sprintf("URL: %s\n", r.URL.String())
	response += fmt.Sprintf("Host: %s\n", r.Host)
	response += fmt.Sprintf("RemoteAddr: %s\n", r.RemoteAddr)
	response += fmt.Sprintf("Content-Length: %d\n", len(body))
	response += fmt.Sprintf("Headers:\n")
	for k, v := range r.Header {
		response += fmt.Sprintf("  %s: %s\n", k, strings.Join(v, ", "))
	}
	response += fmt.Sprintf("\nBody:\n%s\n", string(body))

	written, _ := w.Write([]byte(response))

	s.mu.Lock()
	s.bytesOut += int64(written)
	s.mu.Unlock()
}

func (s *EchoServer) handlePing(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("pong"))
}

func (s *EchoServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	requests := s.requests
	bytesIn := s.bytesIn
	bytesOut := s.bytesOut
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","requests":%d,"bytes_in":%d,"bytes_out":%d}`,
		requests, bytesIn, bytesOut)
}

func (s *EchoServer) handleGenerate204(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// HTTPEchoServer HTTP Echo 服务器（更简单的版本）
type HTTPEchoServer struct {
	port    int
	ln      net.Listener
	running bool
	mu      sync.Mutex
}

// NewHTTPEchoServer 创建 HTTP Echo 服务器
func NewHTTPEchoServer(port int) *HTTPEchoServer {
	return &HTTPEchoServer{port: port}
}

// Start 启动服务器
func (s *HTTPEchoServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.running = true

	go s.serve(ctx)

	return nil
}

// Stop 停止服务器
func (s *HTTPEchoServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// Port 返回端口
func (s *HTTPEchoServer) Port() int {
	return s.port
}

// URL 返回 URL
func (s *HTTPEchoServer) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

func (s *HTTPEchoServer) serve(ctx context.Context) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		go s.handleConn(ctx, conn)
	}
}

func (s *HTTPEchoServer) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// 设置超时
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// 读取 HTTP 请求
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	// 构建响应
	response := fmt.Sprintf("HTTP/1.1 200 OK\r\n")
	response += fmt.Sprintf("Content-Type: text/plain\r\n")
	response += fmt.Sprintf("Connection: close\r\n")
	response += fmt.Sprintf("\r\n")
	response += fmt.Sprintf("Echo: %s %s\n", request.Method, request.URL.String())

	conn.Write([]byte(response))
}

// TCPEchoServer TCP Echo 服务器
type TCPEchoServer struct {
	port    int
	ln      net.Listener
	running bool
	mu      sync.Mutex
}

// NewTCPEchoServer 创建 TCP Echo 服务器
func NewTCPEchoServer(port int) *TCPEchoServer {
	return &TCPEchoServer{port: port}
}

// Start 启动服务器
func (s *TCPEchoServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.running = true

	go s.serve(ctx)

	return nil
}

// Stop 停止服务器
func (s *TCPEchoServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}

// Port 返回端口
func (s *TCPEchoServer) Port() int {
	return s.port
}

// Addr 返回地址
func (s *TCPEchoServer) Addr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.port)
}

func (s *TCPEchoServer) serve(ctx context.Context) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		go s.handleConn(conn)
	}
}

func (s *TCPEchoServer) handleConn(conn net.Conn) {
	defer conn.Close()

	// Echo: 将收到的数据原样返回
	io.Copy(conn, conn)
}