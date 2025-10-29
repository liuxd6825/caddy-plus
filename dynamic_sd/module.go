// package dynamic_sd 实现了 Caddy 的动态上游模块，
// 它可以将服务发现任务委派给不同的提供者（如 Nacos, Consul 等）。
package dynamic_sd

import (
	"fmt"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"

	// 导入你的内部 providers 包
	"github.com/liuxd6825/caddy-plus/internal/providers"
)

func init() {
	caddy.RegisterModule(DynamicSD{})
}

// DynamicSD 是一个 Caddy 动态上游模块，它本身不执行服务发现，
// 而是作为一个容器，将任务委派给一个具体的海服务发现提供者。
type DynamicSD struct {
	// provider 存储了被选中的、实现了 Provider 接口的实例 (例如 NacosProvider)。
	// `json:"-"` 标签防止 Caddy 在 JSON 配置中处理此字段。
	provider providers.Provider `json:"-"`
}

// CaddyModule 返回 Caddy 模块信息。
// 这是将 DynamicSD 注册为 Caddy 模块的关键。
func (DynamicSD) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.reverse_proxy.upstreams.dynamic_sd",
		New: func() caddy.Module { return new(DynamicSD) },
	}
}

// Provision 在 Caddy 加载和初始化配置时被调用。
// 它负责创建 logger 并将其注入到具体的提供者中。
func (d *DynamicSD) Provision(ctx caddy.Context) error {
	if d.provider == nil {
		return fmt.Errorf("no service discovery provider is configured")
	}

	// 使用 'd' (它是一个合法的 caddy.Module) 来创建 logger。
	logger := ctx.Logger(d)

	// 将创建好的 logger 传递给 provider 的 Provision 方法。
	// 这是依赖注入的关键一步。
	return d.provider.Provision(logger)
}

// Validate 确保配置是有效的，它将验证任务委派给提供者。
func (d *DynamicSD) Validate() error {
	if d.provider == nil {
		return fmt.Errorf("no service discovery provider is configured")
	}
	return d.provider.Validate()
}

// Cleanup 在 Caddy 停止或重载配置时被调用。
// 它将清理任务委派给具体的提供者。
func (d *DynamicSD) Cleanup() error {
	if d.provider != nil {
		return d.provider.Cleanup()
	}
	return nil
}

// GetUpstreams 是反向代理的核心调用。
// 它调用内部 provider 的 GetUpstreams 方法来获取最新的服务列表。
func (d *DynamicSD) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	if d.provider == nil {
		return nil, fmt.Errorf("no service discovery provider is configured")
	}
	// 将获取上游列表的任务委派给具体的 provider
	return d.provider.GetUpstreams(r)
}

// UnmarshalCaddyfile 解析 Caddyfile 配置块。
// 这是实现“插件化”和“路由”的核心逻辑。
func (d *DynamicSD) UnmarshalCaddyfile(disp *caddyfile.Dispenser) error {
	for disp.Next() { // 消费模块名 "dynamic_sd"
		if len(disp.RemainingArgs()) > 0 {
			return disp.ArgErr()
		}

		for disp.NextBlock(0) {
			// 期望的唯一指令是 "provider"
			if disp.Val() != "provider" {
				return disp.Errf("unrecognized subdirective '%s', expected 'provider'", disp.Val())
			}

			// "provider" 后面必须跟一个提供者的名字，例如 "nacos", "consul", "mdns"
			if !disp.NextArg() {
				return disp.ArgErr()
			}
			providerName := disp.Val()

			// 使用我们之前编写的工厂函数，根据名称创建提供者实例
			prov, err := providers.NewProvider(providerName)
			if err != nil {
				return disp.Errf("error creating provider '%s': %v", providerName, err)
			}
			d.provider = prov

			// 将 provider 自己的配置块 (e.g., "nacos { ... }") 交给它自己去解析
			if err := d.provider.UnmarshalCaddyfile(disp); err != nil {
				return err
			}
		}
	}
	return nil
}

// 接口符合性检查：确保 DynamicSD 实现了所有必要的 Caddy 接口。
var (
	_ caddy.Module                = (*DynamicSD)(nil)
	_ caddy.Provisioner           = (*DynamicSD)(nil)
	_ caddy.Validator             = (*DynamicSD)(nil)
	_ caddy.CleanerUpper          = (*DynamicSD)(nil)
	_ reverseproxy.UpstreamSource = (*DynamicSD)(nil)
	_ caddyfile.Unmarshaler       = (*DynamicSD)(nil)
)
