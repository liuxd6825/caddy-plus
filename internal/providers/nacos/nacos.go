// package nacos 实现了用于 Caddy 的 Nacos 服务发现提供者。
package nacos

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// NacosProvider 实现了 providers.Provider 接口，
// 用于从 Nacos 动态获取上游服务实例。
type NacosProvider struct {
	// --- 配置字段 ---
	ServerAddr  string   `json:"server_addr,omitempty"`
	ServerPort  uint64   `json:"server_port,omitempty"`
	NamespaceID string   `json:"namespace_id,omitempty"`
	ServiceName string   `json:"service_name,omitempty"`
	GroupName   string   `json:"group_name,omitempty"`
	Clusters    []string `json:"clusters,omitempty"`

	// --- 内部状态 ---
	client    naming_client.INamingClient
	logger    *zap.Logger
	upstreams []*reverseproxy.Upstream
	mu        sync.RWMutex
}

// New 是一个构造函数，返回一个 NacosProvider 的新实例。
// 这是为了满足我们工厂模式的设计。
func New() *NacosProvider {
	return &NacosProvider{
		// 为 GroupName 设置默认值，这很常见
		GroupName: "DEFAULT_GROUP",
	}
}

// Provision 初始化 Nacos 客户端并订阅服务。
func (np *NacosProvider) Provision(logger *zap.Logger) error {
	// 1. 直接将传入的 logger 赋值给结构体字段。
	np.logger = logger
	np.logger.Info("provisioning nacos service discovery provider",
		zap.String("service", np.ServiceName),
		zap.String("group", np.GroupName),
	)

	sc := []constant.ServerConfig{
		*constant.NewServerConfig(np.ServerAddr, np.ServerPort),
	}

	cc := constant.NewClientConfig(
		constant.WithNamespaceId(np.NamespaceID),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithLogDir("/tmp/nacos/log"),
		constant.WithCacheDir("/tmp/nacos/cache"),
		constant.WithLogLevel("warn"),
	)

	var err error
	np.client, err = clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return fmt.Errorf("creating nacos naming client: %v", err)
	}

	// 订阅服务变更
	return np.subscribeToServiceChanges()
}

// subscribeToServiceChanges 设置对 Nacos 服务的订阅。
func (np *NacosProvider) subscribeToServiceChanges() error {
	subscribeParam := &vo.SubscribeParam{
		ServiceName: np.ServiceName,
		GroupName:   np.GroupName,
		Clusters:    np.Clusters,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil {
				np.logger.Error("nacos subscription callback error", zap.Error(err))
				return
			}

			var newUpstreams []*reverseproxy.Upstream
			for _, service := range services {
				// 只选择健康且已启用的实例
				if service.Enable && service.Healthy {
					newUpstreams = append(newUpstreams, &reverseproxy.Upstream{
						Dial: net.JoinHostPort(service.Ip, strconv.FormatUint(service.Port, 10)),
					})
				}
			}

			np.mu.Lock()
			np.upstreams = newUpstreams
			np.mu.Unlock()

			np.logger.Debug("updated upstreams from nacos",
				zap.String("service", np.ServiceName),
				zap.Int("count", len(newUpstreams)),
			)
		},
	}

	if err := np.client.Subscribe(subscribeParam); err != nil {
		return fmt.Errorf("subscribing to nacos service '%s': %v", np.ServiceName, err)
	}

	return nil
}

// Validate 检查必要的配置是否已提供。
func (np *NacosProvider) Validate() error {
	if np.ServerAddr == "" {
		return fmt.Errorf("nacos provider: server_addr is required")
	}
	if np.ServerPort == 0 {
		return fmt.Errorf("nacos provider: server_port is required")
	}
	if np.ServiceName == "" {
		return fmt.Errorf("nacos provider: service_name is required")
	}
	return nil
}

// Cleanup 取消订阅并清理资源。
func (np *NacosProvider) Cleanup() error {
	np.logger.Info("cleaning up nacos provider", zap.String("service", np.ServiceName))
	if np.client == nil {
		return nil
	}

	err := np.client.Unsubscribe(&vo.SubscribeParam{
		ServiceName: np.ServiceName,
		GroupName:   np.GroupName,
	})
	if err != nil {
		return fmt.Errorf("unsubscribing from nacos service '%s': %v", np.ServiceName, err)
	}
	np.client.CloseClient()
	return nil
}

// GetUpstreams 由 Caddy 的反向代理调用以获取当前的上游列表。
// 这个方法必须是线程安全的。
func (np *NacosProvider) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	np.mu.RLock()
	defer np.mu.RUnlock()

	if len(np.upstreams) == 0 {
		return nil, fmt.Errorf("no healthy upstreams available for service: %s", np.ServiceName)
	}
	return np.upstreams, nil
}

// UnmarshalCaddyfile 解析 Nacos 提供者特有的 Caddyfile 配置块。
func (np *NacosProvider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.NextBlock(0) {
		switch d.Val() {
		case "server_addr":
			if !d.NextArg() {
				return d.ArgErr()
			}
			np.ServerAddr = d.Val()
		case "server_port":
			if !d.NextArg() {
				return d.ArgErr()
			}
			port, err := strconv.ParseUint(d.Val(), 10, 64)
			if err != nil {
				return d.Errf("invalid port '%s': %v", d.Val(), err)
			}
			np.ServerPort = port
		case "namespace_id":
			if !d.NextArg() {
				return d.ArgErr()
			}
			np.NamespaceID = d.Val()
		case "service_name":
			if !d.NextArg() {
				return d.ArgErr()
			}
			np.ServiceName = d.Val()
		case "group_name":
			if !d.NextArg() {
				return d.ArgErr()
			}
			np.GroupName = d.Val()
		case "clusters":
			np.Clusters = d.RemainingArgs()
		default:
			return d.Errf("unrecognized nacos subdirective '%s'", d.Val())
		}
	}
	return nil
}
