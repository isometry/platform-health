package k8s

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
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

type Option = func(*Controller)

type Controller struct {
	client dynamic.Interface
	mapper *restmapper.DeferredDiscoveryRESTMapper

	Timeout time.Duration
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *Controller) {
		c.Timeout = timeout
	}
}

func NewController(opts ...Option) (*Controller, error) {
	_inst := new(Controller)
	for _, opt := range opts {
		opt(_inst)
	}

	kCfg, err := getKubeConfig()
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
	return _inst, nil
}

func (c *Controller) GetResource(ctx context.Context, namespace, name string, resource schema.GroupVersionKind, opts metav1.GetOptions) (*unstructured.Unstructured, error) {
	mapping, err := c.mapper.RESTMapping(schema.GroupKind{
		Group: resource.Group,
		Kind:  resource.Kind,
	}, resource.Version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get REST mapping")
	}

	client := c.client.Resource(mapping.Resource).Namespace(namespace)
	return client.Get(ctx, name, opts)
}

func (c *Controller) GetSecret(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*v1.Secret, error) {
	raw, err := c.GetResource(ctx, namespace, name, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	}, opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	var secret v1.Secret
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, &secret); err != nil {
		return nil, errors.Wrap(err, "failed to convert unstructured secret")
	}
	return &secret, nil
}

func (c *Controller) GetConfigMap(ctx context.Context, namespace string, path string, options metav1.GetOptions) (*v1.ConfigMap, error) {
	raw, err := c.GetResource(ctx, namespace, path, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	}, options)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get config map")
	}

	var configMap v1.ConfigMap
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(raw.Object, &configMap); err != nil {
		return nil, errors.Wrap(err, "failed to convert unstructured config map")
	}
	return &configMap, nil
}

func getKubeConfig() (config *rest.Config, err error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_SERVICE_PORT") != "" {
		// in-cluster config
		return rest.InClusterConfig()
	} else {
		// out-of-cluster config
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}
		kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
		return kubeconfig.ClientConfig()
	}
}
