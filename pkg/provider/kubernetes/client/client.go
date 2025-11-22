package client

import (
	"os"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// GetKubeConfig returns a Kubernetes client configuration
func GetKubeConfig() (config *rest.Config, err error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// in-cluster config
		return rest.InClusterConfig()
	}
	// out-of-cluster config
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
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

// KubeClientFactory creates Kubernetes clients
type KubeClientFactory interface {
	GetClients() (*KubeClients, error)
}

// DefaultFactory creates real clients from kubeconfig
type DefaultFactory struct{}

func (f *DefaultFactory) GetClients() (*KubeClients, error) {
	config, err := GetKubeConfig()
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

func (f *MockFactory) GetClients() (*KubeClients, error) {
	return f.Clients, f.Err
}

// ClientFactory is the global factory - replaceable for testing
var ClientFactory KubeClientFactory = &DefaultFactory{}
