package payment

import (
	"context"
	"testing"

	"github.com/neko233-com/pay233-server/internal/config"
)

func TestRegisterConfiguredProvidersExpandsEnvironmentHealth(t *testing.T) {
	registry := NewRegistry()
	err := RegisterConfiguredProviders(registry, []config.ChannelConfig{{
		Name:     "mock",
		Provider: "mock",
		Enabled:  true,
		Environments: map[string]config.ChannelEnvConfig{
			"test": {
				Options: map[string]string{"health_status": "ok"},
			},
			"release": {
				Options: map[string]string{"health_status": "down"},
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	infos := registry.CheckAllHealth(context.Background())
	if len(infos) != 2 {
		t.Fatalf("expected two environment health rows, got %#v", infos)
	}
	byEnv := map[EnvType]ProviderInfo{}
	for _, info := range infos {
		byEnv[info.EnvType] = info
	}
	if byEnv[EnvTypeTest].Health != "ok" {
		t.Fatalf("expected test health ok, got %#v", byEnv[EnvTypeTest])
	}
	if byEnv[EnvTypeRelease].Health != "down" {
		t.Fatalf("expected release health down, got %#v", byEnv[EnvTypeRelease])
	}
}
