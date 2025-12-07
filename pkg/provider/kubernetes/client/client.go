package client

import (
	"fmt"
	"os"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// GetKubeConfig returns a Kubernetes client configuration.
// If context is non-empty, it overrides the current context from kubeconfig.
func GetKubeConfig(context string) (config *rest.Config, err error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		if context != "" {
			return nil, fmt.Errorf("context override not supported when running in-cluster")
		}
		return rest.InClusterConfig()
	}
	// out-of-cluster config
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		configOverrides.CurrentContext = context
	}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return kubeconfig.ClientConfig()
}

// KubeClients holds Kubernetes client implementations
type KubeClients struct {
	Config    *rest.Config
	Dynamic   dynamic.Interface
	Discovery discovery.DiscoveryInterface
	Mapper    meta.RESTMapper
}

// NewFetcher creates a ResourceFetcher bound to these clients
func (c *KubeClients) NewFetcher() *ResourceFetcher {
	return NewFetcher(c)
}

// KubeClientFactory creates Kubernetes clients
type KubeClientFactory interface {
	GetClients(context string) (*KubeClients, error)
}

// DefaultFactory creates real clients from kubeconfig with caching by context
type DefaultFactory struct {
	cache sync.Map // map[string]*KubeClients
}

func (f *DefaultFactory) GetClients(context string) (*KubeClients, error) {
	if cached, ok := f.cache.Load(context); ok {
		return cached.(*KubeClients), nil
	}

	clients, err := f.createClients(context)
	if err != nil {
		return nil, err
	}

	f.cache.Store(context, clients)
	return clients, nil
}

func (f *DefaultFactory) createClients(context string) (*KubeClients, error) {
	config, err := GetKubeConfig(context)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(dc))

	return &KubeClients{
		Config:    config,
		Dynamic:   dynamicClient,
		Discovery: dc,
		Mapper:    mapper,
	}, nil
}

// MockFactory for testing - allows injecting fake clients
type MockFactory struct {
	Clients *KubeClients
	Err     error
}

func (f *MockFactory) GetClients(context string) (*KubeClients, error) {
	return f.Clients, f.Err
}

// ClientFactory is the global factory - replaceable for testing
var ClientFactory KubeClientFactory = &DefaultFactory{}
