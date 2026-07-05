package grpc

import (
	pb "github.com/anixops/v2board/api/grpc/v2boardpb"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/service"
)

// fillNodeConfigResponse 用共享的 service.BuildNodeProtocolConfig 填充 gRPC 的
// NodeConfigResponse, 让 gRPC 节点拿到和 HTTP/订阅端完全一致的协议配置
// (cipher / server_key / flow / tls_settings / network_settings 等)。
//
// node_server.GetConfig 和 config_sync.buildNodeConfigResponse 都调它, 统一逻辑。
func fillNodeConfigResponse(node *model.Node, protocol *model.NodeProtocol) *pb.NodeConfigResponse {
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

	if port, ok := asInt32(cfg["server_port"]); ok {
		resp.ServerPort = port
	}
	if tls, ok := asInt32(cfg["tls"]); ok {
		resp.Tls = tls
	}

	if ts, ok := cfg["tls_settings"].(map[string]any); ok {
		resp.TlsSettings = service.StringifyConfigMap(ts)
	}
	if ns, ok := cfg["network_settings"].(map[string]any); ok {
		resp.NetworkSettings = service.StringifyConfigMap(ns)
	}

	return resp
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// asInt32 处理 BuildNodeProtocolConfig 里 server_port(int) / tls(model.NodeStatus
// 或 int) 等数值字段, 兼容 int / int32 / int64 / float64。
func asInt32(v any) (int32, bool) {
	switch t := v.(type) {
	case int:
		return int32(t), true
	case int32:
		return t, true
	case int64:
		return int32(t), true
	case float64:
		return int32(t), true
	default:
		return 0, false
	}
}
