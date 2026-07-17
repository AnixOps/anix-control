package grpc

import (
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// fillNodeConfigResponse 用共享的 service.BuildNodeProtocolConfig 填充 gRPC 的
// NodeConfigResponse, 让 gRPC 节点拿到和 HTTP/订阅端完全一致的协议配置
// (cipher / server_key / flow / tls_settings / network_settings 等)。
//
// node_server.GetConfig 和 config_sync.buildNodeConfigResponse 都调它, 统一逻辑。
func fillNodeConfigResponse(node *model.Node, protocol *model.NodeProtocol) (*pb.NodeConfigResponse, error) {
	cfg := service.BuildNodeProtocolConfig(node, protocol)

	resp := &pb.NodeConfigResponse{
		SendThrough: "0.0.0.0",
		BaseConfig: &pb.BaseConfig{
			PushInterval: 60,
			PullInterval: 60,
		},
	}

	resp.NodeType = asString(cfg["node_type"])
	resp.Type = asString(cfg["type"])
	resp.Host = asString(cfg["host"])
	resp.ServerName = asString(cfg["server_name"])
	resp.Network = asString(cfg["network"])
	resp.Cipher = asString(cfg["cipher"])
	resp.ServerKey = asString(cfg["server_key"])
	resp.Flow = asString(cfg["flow"])

	if port, ok, err := asInt32("server_port", cfg["server_port"]); err != nil {
		return nil, err
	} else if ok {
		resp.ServerPort = port
	}
	if tls, ok, err := asInt32("tls", cfg["tls"]); err != nil {
		return nil, err
	} else if ok {
		resp.Tls = tls
	}

	if ts, ok := cfg["tls_settings"].(map[string]any); ok {
		resp.TlsSettings = service.StringifyConfigMap(ts)
	}
	if ns, ok := cfg["network_settings"].(map[string]any); ok {
		resp.NetworkSettings = service.StringifyConfigMap(ns)
	}
	resp.Extra = service.StringifyConfigMap(extraNodeConfigFields(cfg))

	return resp, nil
}

func extraNodeConfigFields(cfg map[string]any) map[string]any {
	if len(cfg) == 0 {
		return nil
	}
	skip := map[string]bool{
		"node_type":        true,
		"type":             true,
		"host":             true,
		"server_port":      true,
		"server_name":      true,
		"tls":              true,
		"tls_settings":     true,
		"network":          true,
		"network_settings": true,
		"cipher":           true,
		"flow":             true,
		"server_key":       true,
		"base_config":      true,
		"routes":           true,
		"send_through":     true,
	}
	extra := make(map[string]any)
	for k, v := range cfg {
		if !skip[k] {
			extra[k] = v
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asInt32 处理 BuildNodeProtocolConfig 里 server_port(int) / tls(model.NodeStatus
// 或 int) 等数值字段, 兼容 int / int32 / int64 / float64。
func asInt32(label string, v any) (int32, bool, error) {
	return anyToInt32(label, v)
}
