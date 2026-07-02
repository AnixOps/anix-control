package service

import "fmt"

// AgentDiagnosticActionSpec 描述一个白名单诊断动作允许携带哪些参数，以及是否为变更类操作。
type AgentDiagnosticActionSpec struct {
	Params   []string
	Mutating bool
}

const (
	AgentDiagnosticActionServiceStatus  = "service_status"
	AgentDiagnosticActionServiceRestart = "service_restart"
	AgentDiagnosticActionLogTail        = "log_tail"

	// AgentDiagnosticLogTailMaxLines 限制 log_tail 一次最多返回的行数，避免超大输出。
	AgentDiagnosticLogTailMaxLines   = 1000
	agentDiagnosticLogTailDefaultLines = 100
)

// AgentDiagnosticActions 是终端功能允许下发的全部动作，不接受任意 action 字符串。
var AgentDiagnosticActions = map[string]AgentDiagnosticActionSpec{
	AgentDiagnosticActionServiceStatus:  {Params: []string{"service"}, Mutating: false},
	AgentDiagnosticActionServiceRestart: {Params: []string{"service"}, Mutating: true},
	AgentDiagnosticActionLogTail:        {Params: []string{"service", "lines"}, Mutating: false},
}

// agentDiagnosticAllowedServices 是 service 参数允许的枚举值，不能是任意字符串。
// 注意：这里管理的是 NodeX Agent 所在机器上的服务（目前仅确认 gost 转发进程），
// 与 clean_agent 体系下的 v2forward-agent 二进制/服务无关，不要混用。
var agentDiagnosticAllowedServices = map[string]bool{
	"gost": true,
}

// ValidateAgentDiagnosticTask 校验 action 是否在白名单内，并将 params 归一化为
// 仅包含该 action 允许的字段（多余字段被丢弃，数值做范围裁剪）。
// 返回的 error 可直接展示给管理员。
func ValidateAgentDiagnosticTask(action string, params map[string]any) (map[string]any, error) {
	spec, ok := AgentDiagnosticActions[action]
	if !ok {
		return nil, fmt.Errorf("action %q is not in the diagnostic whitelist", action)
	}

	normalized := map[string]any{}
	for _, key := range spec.Params {
		switch key {
		case "service":
			service, _ := params["service"].(string)
			if !agentDiagnosticAllowedServices[service] {
				return nil, fmt.Errorf("service %q is not allowed", service)
			}
			normalized["service"] = service
		case "lines":
			normalized["lines"] = normalizeAgentDiagnosticLogLines(params["lines"])
		}
	}
	return normalized, nil
}

func normalizeAgentDiagnosticLogLines(raw any) int {
	lines := agentDiagnosticLogTailDefaultLines
	switch v := raw.(type) {
	case float64:
		lines = int(v)
	case int:
		lines = v
	}
	if lines <= 0 {
		lines = agentDiagnosticLogTailDefaultLines
	}
	if lines > AgentDiagnosticLogTailMaxLines {
		lines = AgentDiagnosticLogTailMaxLines
	}
	return lines
}
