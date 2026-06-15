package payment

import (
	"context"
	"fmt"
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
	for _, env := range provider.HealthEnvironments() {
		info := provider.Info()
		info.Name = channel
		info.EnvType = env
		r.health[healthKey(channel, env)] = info
	}
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
	for _, info := range r.health {
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Name == infos[j].Name {
			return envSortRank(infos[i].EnvType) < envSortRank(infos[j].EnvType)
		}
		return infos[i].Name < infos[j].Name
	})
	return infos
}

func (r *Registry) CheckChannelHealth(ctx context.Context, channel string) (ProviderInfo, bool) {
	r.mu.RLock()
	provider, ok := r.providers[channel]
	r.mu.RUnlock()
	if !ok {
		return ProviderInfo{}, false
	}
	envs := provider.HealthEnvironments()
	env := EnvType("")
	if len(envs) > 0 {
		env = envs[0]
	}
	return r.CheckChannelHealthForEnv(ctx, channel, env)
}

func (r *Registry) CheckChannelHealthForEnv(ctx context.Context, channel string, env EnvType) (ProviderInfo, bool) {
	r.mu.RLock()
	provider, ok := r.providers[channel]
	r.mu.RUnlock()
	if !ok {
		return ProviderInfo{}, false
	}
	info := provider.CheckHealth(ctx, env)
	info.Name = channel
	info.EnvType = env
	r.mu.Lock()
	r.health[healthKey(channel, env)] = info
	r.mu.Unlock()
	return info, true
}

func (r *Registry) CheckAllHealth(ctx context.Context) []ProviderInfo {
	channels := r.Channels()
	infos := make([]ProviderInfo, 0, len(channels)*2)
	for _, channel := range channels {
		provider, ok := r.Get(channel)
		if !ok {
			continue
		}
		for _, env := range provider.HealthEnvironments() {
			info, ok := r.CheckChannelHealthForEnv(ctx, channel, env)
			if ok {
				infos = append(infos, info)
			}
		}
	}
	return infos
}

func RegisterConfiguredProviders(registry *Registry, channels []config.ChannelConfig) error {
	for _, channel := range channels {
		if !channel.Enabled {
			continue
		}
		envs := make(map[EnvType]ProviderEnvConfig, len(channel.Environments))
		for rawEnv, envConfig := range channel.Environments {
			env, err := NormalizeEnvType(rawEnv)
			if err != nil {
				return fmt.Errorf("channel %s environment %q: %w", channel.Name, rawEnv, err)
			}
			envs[env] = ProviderEnvConfig{
				Credentials: envConfig.Credentials,
				Options:     envConfig.Options,
			}
		}
		registry.Register(channel.Name, NewConfiguredProviderWithEnvironments(channel.Provider, channel.Credentials, channel.Options, envs))
	}
	return nil
}

func healthKey(channel string, env EnvType) string {
	if env == "" {
		return channel
	}
	return channel + "\x00" + string(env)
}

func envSortRank(env EnvType) int {
	switch env {
	case "":
		return 0
	case EnvTypeTest:
		return 1
	case EnvTypeRelease:
		return 2
	default:
		return 3
	}
}
