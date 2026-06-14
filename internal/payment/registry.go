package payment

import "github.com/neko233-com/pay233-server/internal/config"

type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(channel string, provider Provider) {
	r.providers[channel] = provider
}

func (r *Registry) Get(channel string) (Provider, bool) {
	provider, ok := r.providers[channel]
	return provider, ok
}

func (r *Registry) Channels() []string {
	channels := make([]string, 0, len(r.providers))
	for channel := range r.providers {
		channels = append(channels, channel)
	}
	return channels
}

func (r *Registry) ChannelInfos() []ProviderInfo {
	infos := make([]ProviderInfo, 0, len(r.providers))
	for channel, provider := range r.providers {
		info := provider.Info()
		info.Name = channel
		infos = append(infos, info)
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
