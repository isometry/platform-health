package provider

import (
	"reflect"
	"sync"
)

type ProviderRegistry map[string]reflect.Type

// providers is the internal registry of provider names to their types.
var (
	providers = ProviderRegistry{}
	mu        sync.RWMutex
)

// Register adds a provider to the registry.
func Register(name string, provider Instance) {
	mu.Lock()
	defer mu.Unlock()

	providers[name] = reflect.TypeOf(provider)
}

// List returns a list of registered providers.
func ProviderList() []string {
	mu.RLock()
	defer mu.RUnlock()

	list := make([]string, 0, len(providers))
	for provider := range providers {
		list = append(list, provider)
	}
	return list
}

// RegistryForTesting returns the provider registry for test assertions.
func RegistryForTesting() ProviderRegistry {
	mu.RLock()
	defer mu.RUnlock()
	return providers
}
