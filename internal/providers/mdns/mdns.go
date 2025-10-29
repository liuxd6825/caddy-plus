// package mdns 实现了用于 Caddy 的 mDNS (Bonjour/Zeroconf) 服务发现提供者。
package mdns

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/grandcat/zeroconf"
	"go.uber.org/zap"
)

// MdnsProvider 实现了 providers.Provider 接口，
// 用于通过 mDNS 在本地网络上动态发现上游服务。
type MdnsProvider struct {
	// --- 配置字段 ---
	ServiceName   string        `json:"service_name,omitempty"` // 例如 "_http._tcp"
	Domain        string        `json:"domain,omitempty"`
	BrowseTimeout time.Duration `json:"browse_timeout,omitempty"`

	// --- 内部状态 ---
	logger     *zap.Logger
	upstreams  []*reverseproxy.Upstream
	mu         sync.RWMutex
	cancelFunc context.CancelFunc
}

// New 是一个构造函数，返回一个 MdnsProvider 的新实例。
func New() *MdnsProvider {
	return &MdnsProvider{
		// 设置 mDNS 标准默认值
		Domain:        "local.",
		BrowseTimeout: 5 * time.Second,
	}
}

// Provision 初始化 mDNS 发现 goroutine。
func (mp *MdnsProvider) Provision(logger *zap.Logger) error {
	mp.logger = logger
	mp.logger.Info("provisioning mDNS service discovery provider",
		zap.String("service", mp.ServiceName),
		zap.String("domain", mp.Domain),
	)

	// 创建一个可取消的 context，用于在 Cleanup 时停止 mDNS 浏览器
	var ctx context.Context
	ctx, mp.cancelFunc = context.WithCancel(context.Background())

	// 启动后台 goroutine 来发现和更新服务
	go mp.runDiscovery(ctx)

	return nil
}

// runDiscovery 启动 zeroconf 浏览器并监听服务实例。
func (mp *MdnsProvider) runDiscovery(ctx context.Context) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		mp.logger.Error("failed to initialize mDNS resolver", zap.Error(err))
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)

	// activeServices 用于跟踪当前所有活跃的服务实例
	activeServices := make(map[string]*reverseproxy.Upstream)

	go func() {
		// 这个内部 goroutine 负责从 channel 读取并更新上游列表
		for entry := range entries {
			// 当 TTL 为 0 时，表示服务实例已离开网络
			if entry.TTL == 0 {
				if _, ok := activeServices[entry.Instance]; ok {
					delete(activeServices, entry.Instance)
					mp.logger.Info("mDNS service instance left", zap.String("instance", entry.Instance))
					mp.updateUpstreams(activeServices)
				}
				continue
			}

			// 优先使用 IPv4 地址
			addr := entry.AddrIPv4[0].String()
			if addr == "" && len(entry.AddrIPv6) > 0 {
				addr = entry.AddrIPv6[0].String()
			}
			if addr == "" {
				continue
			}

			upstream := &reverseproxy.Upstream{
				Dial: net.JoinHostPort(addr, strconv.Itoa(entry.Port)),
			}

			activeServices[entry.Instance] = upstream
			mp.logger.Info("mDNS service instance found/updated",
				zap.String("instance", entry.Instance),
				zap.String("address", upstream.Dial),
			)
			mp.updateUpstreams(activeServices)
		}
	}()

	mp.logger.Info("starting mDNS browser...")
	err = resolver.Browse(ctx, mp.ServiceName, mp.Domain, entries)
	if err != nil {
		mp.logger.Error("mDNS browse failed to start", zap.Error(err))
	}

	// Browse 会阻塞直到 context 被取消，当它返回后，我们关闭 channel 来终止上面的 for-range 循环
	close(entries)
	mp.logger.Info("mDNS browser stopped.")
}

// updateUpstreams 是一个线程安全的辅助函数，用于用 map 中的数据更新上游切片。
func (mp *MdnsProvider) updateUpstreams(activeServices map[string]*reverseproxy.Upstream) {
	newUpstreams := make([]*reverseproxy.Upstream, 0, len(activeServices))
	for _, up := range activeServices {
		newUpstreams = append(newUpstreams, up)
	}

	mp.mu.Lock()
	mp.upstreams = newUpstreams
	mp.mu.Unlock()

	mp.logger.Debug("updated upstreams from mDNS", zap.Int("count", len(newUpstreams)))
}

// Validate 检查必要的配置是否已提供。
func (mp *MdnsProvider) Validate() error {
	if mp.ServiceName == "" {
		return fmt.Errorf("mdns provider: service_name is required (e.g., '_http._tcp')")
	}
	return nil
}

// Cleanup 停止 mDNS 浏览器。
func (mp *MdnsProvider) Cleanup() error {
	mp.logger.Info("cleaning up mDNS provider", zap.String("service", mp.ServiceName))
	if mp.cancelFunc != nil {
		mp.cancelFunc()
	}
	return nil
}

// GetUpstreams 由 Caddy 的反向代理调用以获取当前的上游列表。
func (mp *MdnsProvider) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if len(mp.upstreams) == 0 {
		return nil, fmt.Errorf("no mDNS instances available for service: %s", mp.ServiceName)
	}
	return mp.upstreams, nil
}

// UnmarshalCaddyfile 解析 mDNS 提供者特有的 Caddyfile 配置块。
func (mp *MdnsProvider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.NextBlock(0) {
		switch d.Val() {
		case "service_name":
			if !d.NextArg() {
				return d.ArgErr()
			}
			mp.ServiceName = d.Val()
		case "domain":
			if !d.NextArg() {
				return d.ArgErr()
			}
			mp.Domain = d.Val()
		case "browse_timeout":
			if !d.NextArg() {
				return d.ArgErr()
			}
			dur, err := caddy.ParseDuration(d.Val())
			if err != nil {
				return d.Errf("invalid duration for browse_timeout: %v", err)
			}
			mp.BrowseTimeout = dur
		default:
			return d.Errf("unrecognized mdns subdirective '%s'", d.Val())
		}
	}
	return nil
}
