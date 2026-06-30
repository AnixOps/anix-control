package payment

import (
	"fmt"
	"sort"
	"sync"
)

// registry 是全局支付网关注册表，按 Type() 索引。
var (
	registryMu sync.RWMutex
	registry   = make(map[string]Gateway)
)

// Register 注册一个支付网关插件。通常在网关实现文件的 init() 中调用。
// 重复注册同一 type 会 panic，以便在启动期暴露配置错误。
func Register(g Gateway) {
	registryMu.Lock()
	defer registryMu.Unlock()

	t := g.Type()
	if t == "" {
		panic("payment: gateway Type() must not be empty")
	}
	if _, exists := registry[t]; exists {
		panic(fmt.Sprintf("payment: gateway %q already registered", t))
	}
	registry[t] = g
}

// Get 按类型返回已注册的网关，第二个返回值表示是否存在。
func Get(gatewayType string) (Gateway, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	g, ok := registry[gatewayType]
	return g, ok
}

// Types 返回所有已注册的网关类型，按字母序排列（便于测试和展示）。
func Types() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}
