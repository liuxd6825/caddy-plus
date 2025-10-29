// package consul 实现了用于 Caddy 的 Consul 服务发现提供者。
package consul

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	consulApi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// ConsulProvider 实现了 providers.Provider 接口，
// 用于从 Consul 动态获取上游服务实例。
type ConsulProvider struct {
	// --- 配置字段 ---
	Address      string        `json:"address,omitempty"`
	ServiceName  string        `json:"service_name,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	PassingOnly  bool          `json:"passing_only,omitempty"`
	PollInterval time.Duration `json:"poll_interval,omitempty"`

	// --- 内部状态 ---
	client    *consulApi.Client
	logger    *zap.Logger
	upstreams []*reverseproxy.Upstream
	mu        sync.RWMutex
	stopChan  chan struct{}
}

// New 是一个构造函数，返回一个 ConsulProvider 的新实例。
func New() *ConsulProvider {
	return &ConsulProvider{
		// 设置合理的默认值
		PassingOnly:  true,
		PollInterval: 10 * time.Second, // 默认每 10 秒轮询一次
	}
}

// Provision 初始化 Consul 客户端并启动后台轮询 goroutine。
func (cp *ConsulProvider) Provision(logger *zap.Logger) error {
	cp.logger = logger
	cp.logger.Info("provisioning consul service discovery provider",
		zap.String("service", cp.ServiceName),
		zap.String("address", cp.Address),
	)
	cp.stopChan = make(chan struct{})

	// 创建 Consul 客户端
	config := consulApi.DefaultConfig()
	if cp.Address != "" {
		config.Address = cp.Address
	}
	var err error
	cp.client, err = consulApi.NewClient(config)
	if err != nil {
		return fmt.Errorf("creating consul client: %v", err)
	}

	// 立即执行一次服务获取，以确保在 Caddy 启动时就有上游可用
	if err := cp.updateUpstreams(); err != nil {
		cp.logger.Error("initial fetch from consul failed", zap.Error(err))
		// 我们不在这里返回错误，因为网络可能是暂时问题，后台轮询可能会恢复
	}

	// 启动后台 goroutine 定期更新服务列表
	go cp.watchServiceChanges()

	return nil
}

// updateUpstreams 从 Consul 获取服务实例并更新内部列表。
func (cp *ConsulProvider) updateUpstreams() error {
	serviceEntries, _, err := cp.client.Health().Service(cp.ServiceName, "", cp.PassingOnly, nil)
	if err != nil {
		return fmt.Errorf("querying consul for service '%s': %v", cp.ServiceName, err)
	}

	var newUpstreams []*reverseproxy.Upstream
	for _, entry := range serviceEntries {
		// 地址优先使用 Service.Address，如果为空则回退到 Node.Address
		addr := entry.Service.Address
		if addr == "" {
			addr = entry.Node.Address
		}

		newUpstreams = append(newUpstreams, &reverseproxy.Upstream{
			Dial: net.JoinHostPort(addr, strconv.Itoa(entry.Service.Port)),
		})
	}

	cp.mu.Lock()
	cp.upstreams = newUpstreams
	cp.mu.Unlock()

	cp.logger.Debug("updated upstreams from consul",
		zap.String("service", cp.ServiceName),
		zap.Int("count", len(newUpstreams)),
	)
	return nil
}

// watchServiceChanges 是一个在后台运行的循环，定期从 Consul 拉取更新。
func (cp *ConsulProvider) watchServiceChanges() {
	ticker := time.NewTicker(cp.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := cp.updateUpstreams(); err != nil {
				cp.logger.Error("failed to update upstreams from consul", zap.Error(err))
			}
		case <-cp.stopChan:
			cp.logger.Info("stopping consul service watcher", zap.String("service", cp.ServiceName))
			return
		}
	}
}

// Validate 检查必要的配置是否已提供。
func (cp *ConsulProvider) Validate() error {
	if cp.ServiceName == "" {
		return fmt.Errorf("consul provider: service_name is required")
	}
	return nil
}

// Cleanup 停止后台 goroutine 并清理资源。
func (cp *ConsulProvider) Cleanup() error {
	cp.logger.Info("cleaning up consul provider", zap.String("service", cp.ServiceName))
	if cp.stopChan != nil {
		close(cp.stopChan)
	}
	return nil
}

// GetUpstreams 由 Caddy 的反向代理调用以获取当前的上游列表。
func (cp *ConsulProvider) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	if len(cp.upstreams) == 0 {
		return nil, fmt.Errorf("no healthy upstreams available for service: %s", cp.ServiceName)
	}
	return cp.upstreams, nil
}

// UnmarshalCaddyfile 解析 Consul 提供者特有的 Caddyfile 配置块。
func (cp *ConsulProvider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.NextBlock(0) {
		switch d.Val() {
		case "address":
			if !d.NextArg() {
				return d.ArgErr()
			}
			cp.Address = d.Val()
		case "service_name":
			if !d.NextArg() {
				return d.ArgErr()
			}
			cp.ServiceName = d.Val()
		case "tags":
			cp.Tags = d.RemainingArgs()
		case "passing_only":
			if !d.NextArg() {
				return d.ArgErr()
			}
			val, err := strconv.ParseBool(d.Val())
			if err != nil {
				return d.Errf("invalid boolean for passing_only: %v", err)
			}
			cp.PassingOnly = val
		case "poll_interval":
			if !d.NextArg() {
				return d.ArgErr()
			}
			dur, err := caddy.ParseDuration(d.Val())
			if err != nil {
				return d.Errf("invalid duration for poll_interval: %v", err)
			}
			cp.PollInterval = dur
		default:
			return d.Errf("unrecognized consul subdirective '%s'", d.Val())
		}
	}
	return nil
}
