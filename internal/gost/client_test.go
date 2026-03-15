package gost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := &Config{
		Host:     "http://localhost:8080",
		APIToken: "test-token",
		Timeout:  5 * time.Second,
	}

	client := NewClient(cfg)
	if client == nil {
		t.Fatal("client should not be nil")
	}

	if client.baseURL != cfg.Host {
		t.Errorf("baseURL = %s, want %s", client.baseURL, cfg.Host)
	}

	if client.authPass != cfg.APIToken {
		t.Errorf("authPass = %s, want %s", client.authPass, cfg.APIToken)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	cfg := &Config{
		Host:     "http://localhost:8080",
		APIToken: "test-token",
	}

	client := NewClient(cfg)
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("default timeout = %v, want 10s", client.httpClient.Timeout)
	}
}

func TestClient_GetServices(t *testing.T) {
	// 创建模拟服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/services" {
			t.Errorf("path = %s, want /api/config/services", r.URL.Path)
		}

		resp := struct {
			Data ServiceListResponse `json:"data"`
		}{
			Data: ServiceListResponse{
				Count: 2,
				List: []*ServiceConfig{
					{Name: "service-1", Addr: ":8080"},
					{Name: "service-2", Addr: ":8081"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Host:     server.URL,
		APIToken: "test",
	})

	services, err := client.GetServices(context.Background())
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if services.Count != 2 {
		t.Errorf("count = %d, want 2", services.Count)
	}

	if len(services.List) != 2 {
		t.Errorf("list length = %d, want 2", len(services.List))
	}
}

func TestClient_GetService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/services/test-service" {
			t.Errorf("path = %s, want /api/config/services/test-service", r.URL.Path)
		}

		resp := struct {
			Data *ServiceConfig `json:"data"`
		}{
			Data: &ServiceConfig{
				Name: "test-service",
				Addr: ":8080",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	svc, err := client.GetService(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}

	if svc.Name != "test-service" {
		t.Errorf("name = %s, want test-service", svc.Name)
	}
}

func TestClient_CreateService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/config/services" {
			t.Errorf("path = %s, want /api/config/services", r.URL.Path)
		}

		var req struct {
			Data ServiceConfig `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}

		if req.Data.Name != "new-service" {
			t.Errorf("name = %s, want new-service", req.Data.Name)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.CreateService(context.Background(), &ServiceConfig{
		Name: "new-service",
		Addr: ":9090",
	})
	if err != nil {
		t.Fatalf("CreateService failed: %v", err)
	}
}

func TestClient_UpdateService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/api/config/services/test-service" {
			t.Errorf("path = %s, want /api/config/services/test-service", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.UpdateService(context.Background(), "test-service", &ServiceConfig{
		Name: "test-service",
		Addr: ":9090",
	})
	if err != nil {
		t.Fatalf("UpdateService failed: %v", err)
	}
}

func TestClient_DeleteService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/config/services/test-service" {
			t.Errorf("path = %s, want /api/config/services/test-service", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.DeleteService(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("DeleteService failed: %v", err)
	}
}

func TestClient_GetStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stats" {
			t.Errorf("path = %s, want /api/stats", r.URL.Path)
		}

		resp := StatsResponse{
			Services: []ServiceStats{
				{
					Name: "service-1",
					Addr: ":8080",
					Current: CurrentStats{
						Connections: 10,
						InBytes:     1024,
						OutBytes:    2048,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	stats, err := client.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if len(stats.Services) != 1 {
		t.Errorf("services length = %d, want 1", len(stats.Services))
	}

	if stats.Services[0].Current.Connections != 10 {
		t.Errorf("connections = %d, want 10", stats.Services[0].Current.Connections)
	}
}

func TestClient_HealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/services" {
			t.Errorf("path = %s, want /api/config/services", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestClient_HealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck should fail with 500 error")
	}
}

func TestClient_CreateChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/config/chains" {
			t.Errorf("path = %s, want /api/config/chains", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.CreateChain(context.Background(), &ChainConfig{
		Name: "test-chain",
		Hops: []string{"hop-1"},
	})
	if err != nil {
		t.Fatalf("CreateChain failed: %v", err)
	}
}

func TestClient_Reload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/config/reload" {
			t.Errorf("path = %s, want /api/config/reload", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.Reload(context.Background())
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	_, err := client.GetServices(context.Background())
	if err == nil {
		t.Fatal("GetServices should fail with 404 error")
	}
}

func TestClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // 模拟延迟
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Host:    server.URL,
		Timeout: 10 * time.Millisecond, // 极短的超时
	})

	err := client.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck should timeout")
	}
}

func TestClient_GetChains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config/chains" {
			t.Errorf("path = %s, want /api/config/chains", r.URL.Path)
		}

		resp := struct {
			Data struct {
				List []*ChainConfig `json:"list"`
			} `json:"data"`
		}{}
		resp.Data.List = []*ChainConfig{
			{Name: "chain-1", Hops: []string{"hop-1"}},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	chains, err := client.GetChains(context.Background())
	if err != nil {
		t.Fatalf("GetChains failed: %v", err)
	}

	if len(chains) != 1 {
		t.Errorf("chains length = %d, want 1", len(chains))
	}

	if chains[0].Name != "chain-1" {
		t.Errorf("chain name = %s, want chain-1", chains[0].Name)
	}
}

func TestClient_DeleteChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/config/chains/test-chain" {
			t.Errorf("path = %s, want /api/config/chains/test-chain", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.DeleteChain(context.Background(), "test-chain")
	if err != nil {
		t.Fatalf("DeleteChain failed: %v", err)
	}
}

func TestClient_CreateHop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/config/hops" {
			t.Errorf("path = %s, want /api/config/hops", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.CreateHop(context.Background(), &HopConfig{
		Name: "test-hop",
		Nodes: []HopNode{
			{Name: "node-1", Addr: "192.168.1.1:8080"},
		},
	})
	if err != nil {
		t.Fatalf("CreateHop failed: %v", err)
	}
}

func TestClient_DeleteHop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/config/hops/test-hop" {
			t.Errorf("path = %s, want /api/config/hops/test-hop", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	err := client.DeleteHop(context.Background(), "test-hop")
	if err != nil {
		t.Fatalf("DeleteHop failed: %v", err)
	}
}

func TestClient_GetServiceStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stats/services/test-service" {
			t.Errorf("path = %s, want /api/stats/services/test-service", r.URL.Path)
		}

		stats := ServiceStats{
			Name: "test-service",
			Addr: ":8080",
			Current: CurrentStats{
				Connections: 5,
				InBytes:     1024,
				OutBytes:    2048,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL})

	stats, err := client.GetServiceStats(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("GetServiceStats failed: %v", err)
	}

	if stats.Name != "test-service" {
		t.Errorf("name = %s, want test-service", stats.Name)
	}

	if stats.Current.Connections != 5 {
		t.Errorf("connections = %d, want 5", stats.Current.Connections)
	}
}

func TestClient_BasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			t.Error("Basic auth not set")
		}
		if username != "" {
			t.Errorf("username = %s, want empty", username)
		}
		if password != "test-token" {
			t.Errorf("password = %s, want test-token", password)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{
		Host:     server.URL,
		APIToken: "test-token",
	})

	_ = client.HealthCheck(context.Background())
}

func TestServiceConfig_JSON(t *testing.T) {
	svc := &ServiceConfig{
		Name: "test-service",
		Addr: ":8080",
		Handler: &HandlerConfig{
			Type: "tcp",
			Auth: &AuthConfig{
				Username: "user",
				Password: "pass",
			},
		},
		Listener: &ListenerConfig{
			Type: "tcp",
			TLS: &TLSConfig{
				CertFile:   "/path/to/cert",
				KeyFile:    "/path/to/key",
				ServerName: "example.com",
			},
		},
		Forwarder: &ForwarderConfig{
			Nodes: []ForwarderNode{
				{Name: "node-1", Addr: "192.168.1.1:80"},
			},
			Selector: &SelectorConfig{
				Strategy:    "round",
				MaxFails:    3,
				FailTimeout: "30s",
			},
		},
	}

	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ServiceConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "test-service" {
		t.Errorf("name = %s, want test-service", decoded.Name)
	}

	if decoded.Handler.Type != "tcp" {
		t.Errorf("handler type = %s, want tcp", decoded.Handler.Type)
	}
}

func TestChainConfig_JSON(t *testing.T) {
	chain := &ChainConfig{
		Name: "test-chain",
		Hops: []string{"hop-1", "hop-2"},
		Selector: &SelectorConfig{
			Strategy: "round",
		},
	}

	data, err := json.Marshal(chain)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ChainConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "test-chain" {
		t.Errorf("name = %s, want test-chain", decoded.Name)
	}

	if len(decoded.Hops) != 2 {
		t.Errorf("hops length = %d, want 2", len(decoded.Hops))
	}
}

func TestHopConfig_JSON(t *testing.T) {
	hop := &HopConfig{
		Name: "test-hop",
		Nodes: []HopNode{
			{
				Name: "node-1",
				Addr: "192.168.1.1:8080",
				Connector: &ConnectorConfig{
					Type: "socks5",
					Auth: &AuthConfig{
						Username: "user",
						Password: "pass",
					},
				},
				Dialer: &DialerConfig{
					Type: "tcp",
				},
			},
		},
	}

	data, err := json.Marshal(hop)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded HopConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Name != "test-hop" {
		t.Errorf("name = %s, want test-hop", decoded.Name)
	}

	if len(decoded.Nodes) != 1 {
		t.Errorf("nodes length = %d, want 1", len(decoded.Nodes))
	}
}

func TestStatsResponse_JSON(t *testing.T) {
	stats := &StatsResponse{
		Services: []ServiceStats{
			{
				Name: "service-1",
				Addr: ":8080",
				Current: CurrentStats{
					Connections: 10,
					InBytes:     1024,
					OutBytes:    2048,
				},
				Total: TotalStats{
					Connections: 100,
					InBytes:     10240,
					OutBytes:    20480,
				},
			},
		},
	}

	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded StatsResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Services) != 1 {
		t.Errorf("services length = %d, want 1", len(decoded.Services))
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&Config{Host: server.URL, Timeout: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.HealthCheck(ctx)
	if err == nil {
		t.Fatal("HealthCheck should fail with cancelled context")
	}
}