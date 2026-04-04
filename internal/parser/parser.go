package parser

import (
	"github.com/anixops/v2board/internal/model"
)

// Parser 订阅解析器接口
// 用于解析各种订阅格式并转换为统一的 ParsedNode 格式
type Parser interface {
	// Parse 解析订阅内容
	Parse(content []byte) ([]*model.ParsedNode, error)

	// Detect 检测内容是否为该格式
	Detect(content []byte) bool

	// Name 获取解析器名称
	Name() string
}

// Formatter 订阅格式化器接口
// 用于将 ParsedNode 列表转换为各种订阅格式
type Formatter interface {
	// Format 格式化节点列表
	Format(nodes []*model.ParsedNode, ctx *model.TemplateRenderContext) ([]byte, error)

	// ContentType 获取 MIME 类型
	ContentType() string

	// FileExtension 获取文件扩展名
	FileExtension() string

	// Name 获取格式化器名称
	Name() string
}

// Registry 解析器和格式化器注册表
type Registry struct {
	parsers    []Parser
	formatters map[model.SubscriptionFormat]Formatter
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	r := &Registry{
		parsers:    make([]Parser, 0),
		formatters: make(map[model.SubscriptionFormat]Formatter),
	}

	// 注册默认解析器
	r.RegisterParser(&Base64Parser{})
	r.RegisterParser(&ClashParser{})
	r.RegisterParser(&SIP008Parser{})

	// 注册默认格式化器
	r.RegisterFormatter(model.FormatV2Ray, &V2RayFormatter{})
	r.RegisterFormatter(model.FormatClash, &ClashFormatter{})
	r.RegisterFormatter(model.FormatStash, &StashFormatter{})
	r.RegisterFormatter(model.FormatEgern, &EgernFormatter{})
	r.RegisterFormatter(model.FormatSurge, &SurgeFormatter{})
	r.RegisterFormatter(model.FormatLoon, &LoonFormatter{})
	r.RegisterFormatter(model.FormatJSON, &JSONFormatter{})
	r.RegisterFormatter(model.FormatBase64JSON, &Base64JSONFormatter{})
	r.RegisterFormatter(model.FormatShadowrocket, &ShadowrocketFormatter{})
	r.RegisterFormatter(model.FormatQuantumultX, &QuantumultXFormatter{})
	r.RegisterFormatter(model.FormatSingBox, &SingBoxFormatter{})

	return r
}

// RegisterParser 注册解析器
func (r *Registry) RegisterParser(p Parser) {
	r.parsers = append(r.parsers, p)
}

// RegisterFormatter 注册格式化器
func (r *Registry) RegisterFormatter(format model.SubscriptionFormat, f Formatter) {
	r.formatters[format] = f
}

// AutoParse 自动检测格式并解析
func (r *Registry) AutoParse(content []byte) ([]*model.ParsedNode, error) {
	for _, p := range r.parsers {
		if p.Detect(content) {
			return p.Parse(content)
		}
	}
	// 尝试所有解析器
	for _, p := range r.parsers {
		nodes, err := p.Parse(content)
		if err == nil && len(nodes) > 0 {
			return nodes, nil
		}
	}
	return nil, ErrUnknownFormat
}

// GetFormatter 获取格式化器
func (r *Registry) GetFormatter(format model.SubscriptionFormat) (Formatter, bool) {
	f, ok := r.formatters[format]
	return f, ok
}

// GetParser 根据名称获取解析器
func (r *Registry) GetParser(name string) Parser {
	for _, p := range r.parsers {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// 错误定义
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrUnknownFormat   Error = "unknown subscription format"
	ErrInvalidContent  Error = "invalid subscription content"
	ErrParseError      Error = "parse error"
	ErrUnsupportedType Error = "unsupported protocol type"
)

// 全局注册表实例
var defaultRegistry = NewRegistry()

// GetDefaultRegistry 获取默认注册表
func GetDefaultRegistry() *Registry {
	return defaultRegistry
}
