package k8s

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	v1core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// Option is a functional option for the controller
type Option = func(*Controller)

// Controller is a Kubernetes controller
type Controller struct {
	client           dynamic.Interface
	mapper           *restmapper.DeferredDiscoveryRESTMapper
	defaultNamespace string

	Timeout time.Duration
}

// WithTimeout sets the timeout for the controller
func WithTimeout(timeout time.Duration) Option {
	return func(c *Controller) {
		c.Timeout = timeout
	}
}

// NewController creates a new Kubernetes controller
func NewController(opts ...Option) (*Controller, error) {
	_inst := new(Controller)
	for _, opt := range opts {
		opt(_inst)
	}

	defNs, kCfg, err := getKubeConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubeconfig")
	}

	kCfg.Timeout = _inst.Timeout
	clt, err := dynamic.NewForConfig(kCfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create dynamic client")
	}

	dc, _ := discovery.NewDiscoveryClientForConfig(kCfg)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	_inst.mapper = mapper
	_inst.client = clt
	_inst.defaultNamespace = defNs
	return _inst, nil
}

// GetResource gets a resource from the Kubernetes cluster
func (c *Controller) GetResource(ctx context.Context, namespace, name string, resource schema.GroupVersionKind, opts metav1.GetOptions) (*unstructured.Unstructured, error) {
	mapping, err := c.mapper.RESTMapping(schema.GroupKind{
		Group: resource.Group,
		Kind:  resource.Kind,
	}, resource.Version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get REST mapping")
	}
	if namespace == "" {
		namespace = c.defaultNamespace
	}

	client := c.client.Resource(mapping.Resource).Namespace(namespace)
	return client.Get(ctx, name, opts)
}

// GetSecret gets a secret from the Kubernetes cluster
func (c *Controller) GetSecret(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1core.Secret, error) {

	raw, err := c.GetResource(ctx, namespace, name, new(v1core.Secret).GroupVersionKind(), opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	var secret v1core.Secret
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, &secret); err != nil {
		return nil, errors.Wrap(err, "failed to convert unstructured secret")
	}
	return &secret, nil
}

// GetConfigMap gets a config map from the Kubernetes cluster
func (c *Controller) GetConfigMap(ctx context.Context, namespace string, name string, options metav1.GetOptions) (*v1core.ConfigMap, error) {
	raw, err := c.GetResource(ctx, namespace, name, new(v1core.ConfigMap).GroupVersionKind(), options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get config map")
	}

	var configMap v1core.ConfigMap
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, &configMap); err != nil {
		return nil, errors.Wrap(err, "failed to convert unstructured config map")
	}
	return &configMap, nil
}

// getKubeConfig returns the Kubernetes configuration and configured default namespace
func getKubeConfig() (defaultNamespace string, config *rest.Config, err error) {
	defaultNamespace = metav1.NamespaceDefault
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// in-cluster config
		config, err = rest.InClusterConfig()
		return
	} else {
		// out-of-cluster config
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		defaultNamespace, _, err = kubeConfig.Namespace()
		if err != nil {
			return "", nil, errors.Wrap(err, "failed to get default namespace")
		}
		config, err = kubeConfig.ClientConfig()
		return
	}
}
