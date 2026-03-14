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