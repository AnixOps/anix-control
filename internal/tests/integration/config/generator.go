package config

import (
	"encoding/json"
	"fmt"
)

// Generator 配置生成器接口
type Generator interface {
	// Name 返回生成器名称
	Name() string

	// Generate 生成客户端配置
	Generate(cfg *ClientConfig) ([]byte, error)

	// GenerateFromScenario 从测试场景生成配置
	GenerateFromScenario(scenario TestScenario, server ServerConfig, user UserConfig) ([]byte, error)

	// SupportedProtocols 返回支持的协议列表
	SupportedProtocols() []Protocol

	// IsProtocolSupported 检查协议是否支持
	IsProtocolSupported(p Protocol) bool
}

// GeneratorRegistry 配置生成器注册表
type GeneratorRegistry struct {
	generators map[string]Generator
}

// NewGeneratorRegistry 创建新的生成器注册表
func NewGeneratorRegistry() *GeneratorRegistry {
	return &GeneratorRegistry{
		generators: make(map[string]Generator),
	}
}

// Register 注册生成器
func (r *GeneratorRegistry) Register(g Generator) {
	r.generators[g.Name()] = g
}

// Get 获取生成器
func (r *GeneratorRegistry) Get(name string) (Generator, error) {
	g, ok := r.generators[name]
	if !ok {
		return nil, fmt.Errorf("generator not found: %s", name)
	}
	return g, nil
}

// List 列出所有生成器
func (r *GeneratorRegistry) List() []string {
	names := make([]string, 0, len(r.generators))
	for name := range r.generators {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry 默认注册表
var DefaultRegistry = NewGeneratorRegistry()

func init() {
	// 注册 Xray 生成器
	DefaultRegistry.Register(NewXrayGenerator())
	// 注册 Mihomo 生成器
	DefaultRegistry.Register(NewMihomoGenerator())
}

// GenerateConfig 生成配置（便捷方法）
func GenerateConfig(generatorName string, cfg *ClientConfig) ([]byte, error) {
	g, err := DefaultRegistry.Get(generatorName)
	if err != nil {
		return nil, err
	}
	return g.Generate(cfg)
}

// ToJSON 辅助函数：转换为格式化的 JSON
func ToJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// ToJSONCompact 辅助函数：转换为紧凑的 JSON
func ToJSONCompact(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}