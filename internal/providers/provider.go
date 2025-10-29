// package providers 为 Caddy 动态服务发现模块定义了统一的接口和工厂。
package providers

import (
	"fmt"
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/liuxd6825/caddy-plus/internal/providers/consul"
	"github.com/liuxd6825/caddy-plus/internal/providers/mdns"
	"go.uber.org/zap"

	// 导入具体的提供者实现
	// 请将 "github.com/your-username/caddy-dynamic-sd" 替换为你的实际模块路径
	"github.com/liuxd6825/caddy-plus/internal/providers/nacos"
	// "github.com/your-username/caddy-dynamic-sd/internal/providers/consul" // <-- 将来添加 Consul 时取消注释
)

// Provider 是所有服务发现提供者必须实现的接口。
// 它组合了多个 Caddy 标准接口以及我们自定义的 Provision 方法，
// 确保每个提供者都能完整地集成到 Caddy 的生命周期和配置流程中。
type Provider interface {
	// Provision 使用从主模块传入的 logger 来初始化提供者。
	Provision(logger *zap.Logger) error

	// caddy.CleanerUpper 接口用于资源清理。
	// 当 Caddy 关闭或重载时，此方法被调用以关闭连接、停止 goroutine 等。
	caddy.CleanerUpper

	// caddy.Validator 接口用于验证配置是否有效。
	caddy.Validator

	// caddyfile.Unmarshaler 接口使提供者能够解析其自身的 Caddyfile 配置块。
	caddyfile.Unmarshaler

	// reverseproxy.UpstreamSource 是核心接口，用于向 Caddy 的反向代理提供上游服务列表。
	reverseproxy.UpstreamSource
}

// NewProvider 是一个工厂函数，根据给定的名称创建并返回一个具体的 Provider 实例。
// 这使得主模块可以动态地选择和实例化服务发现后端。
func NewProvider(name string) (Provider, error) {
	switch name {
	case "nacos":
		// 返回一个新的 Nacos 提供者实例
		return nacos.New(), nil

	case "consul":
		// 返回一个新的 Consul 提供者实例
		return consul.New(), nil

	case "mdns":
		// 返回一个新的 mDNS 提供者实例
		return mdns.New(), nil

	default:
		// 如果提供者名称未知，返回一个错误
		return nil, fmt.Errorf("unknown service discovery provider: '%s'. supported providers are: nacos, consul, mdns", name)
	}
}
