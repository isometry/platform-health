package provider

import (
	"reflect"
	"sync"
)

type ProviderRegistry map[string]reflect.Type

// Providers is a map of provider names to their types, used for registering providers.
var (
	Providers = ProviderRegistry{}
	mu        sync.RWMutex
)

// Register adds a provider to the registry.
func Register(name string, provider Instance) {
	mu.Lock()
	defer mu.Unlock()

	Providers[name] = reflect.TypeOf(provider)
}

// List returns a list of registered providers.
func ProviderList() []string {
	mu.RLock()
	defer mu.RUnlock()

	providers := make([]string, 0, len(Providers))
	for provider := range Providers {
		providers = append(providers, provider)
	}
	return providers
}
