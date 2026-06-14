package payment

import (
	"context"
	"sort"
	"sync"

	"github.com/neko233-com/pay233-server/internal/config"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	health    map[string]ProviderInfo
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		health:    make(map[string]ProviderInfo),
	}
}

func (r *Registry) Register(channel string, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[channel] = provider
	info := provider.Info()
	info.Name = channel
	r.health[channel] = info
}

func (r *Registry) Get(channel string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[channel]
	return provider, ok
}

func (r *Registry) Channels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	channels := make([]string, 0, len(r.providers))
	for channel := range r.providers {
		channels = append(channels, channel)
	}
	sort.Strings(channels)
	return channels
}

func (r *Registry) ChannelInfos() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ProviderInfo, 0, len(r.health))
	for channel, info := range r.health {
		info.Name = channel
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

func (r *Registry) CheckChannelHealth(ctx context.Context, channel string) (ProviderInfo, bool) {
	r.mu.RLock()
	provider, ok := r.providers[channel]
	r.mu.RUnlock()
	if !ok {
		return ProviderInfo{}, false
	}
	info := provider.CheckHealth(ctx)
	info.Name = channel
	r.mu.Lock()
	r.health[channel] = info
	r.mu.Unlock()
	return info, true
}

func (r *Registry) CheckAllHealth(ctx context.Context) []ProviderInfo {
	channels := r.Channels()
	infos := make([]ProviderInfo, 0, len(channels))
	for _, channel := range channels {
		info, ok := r.CheckChannelHealth(ctx, channel)
		if ok {
			infos = append(infos, info)
		}
	}
	return infos
}

func RegisterConfiguredProviders(registry *Registry, channels []config.ChannelConfig) error {
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		registry.Register(channel.Name, NewConfiguredProvider(channel.Provider, channel.Options))
	}
	return nil
}
