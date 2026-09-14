package client

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	k8sclient "github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

// StatusDeployed is exported for use in tests and provider
const StatusDeployed = common.StatusDeployed

// StatusRunner abstracts the helm status action for testing
type StatusRunner interface {
	Run(ctx context.Context, name string) (*release.Release, error)
}

// HelmClientFactory creates helm status runners
type HelmClientFactory interface {
	GetStatusRunner(kubeContext, namespace string, log *slog.Logger) (StatusRunner, error)
}

// DefaultHelmFactory creates real helm clients using kubernetes config
type DefaultHelmFactory struct{}

// configFlagsFor builds the ConfigFlags helm uses for context and namespace
// resolution. Context is set rather than individual credential fields so
// inline client-certificate and CA data resolve in full.
func configFlagsFor(kubeContext, namespace string) *genericclioptions.ConfigFlags {
	kubeConfig := genericclioptions.NewConfigFlags(false)
	if kubeContext != "" {
		kubeConfig.Context = &kubeContext
	}
	kubeConfig.Namespace = &namespace
	return kubeConfig
}

// restClientGetterFor resolves the rest.Config through the kubernetes
// provider's GetKubeConfig, so helm talks to the same cluster with the same
// rate limits, and hands it to cli-runtime in place of its own resolution.
func restClientGetterFor(kubeContext, namespace string) (*genericclioptions.ConfigFlags, error) {
	cfg, err := k8sclient.GetKubeConfig(kubeContext)
	if err != nil {
		return nil, err
	}
	return configFlagsFor(kubeContext, namespace).WithWrapConfigFn(func(*rest.Config) *rest.Config {
		return rest.CopyConfig(cfg)
	}), nil
}

func (f *DefaultHelmFactory) GetStatusRunner(kubeContext, namespace string, log *slog.Logger) (StatusRunner, error) {
	kubeConfig, err := restClientGetterFor(kubeContext, namespace)
	if err != nil {
		return nil, err
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(kubeConfig, namespace, "secret"); err != nil {
		return nil, err
	}

	// Use List action with optimizations instead of Status for better performance
	// with large release histories
	listAction := action.NewList(actionConfig)
	listAction.StateMask = action.ListAll
	listAction.Limit = 1
	listAction.Sort = action.ByDateDesc

	return &listRunner{
		action:    listAction,
		namespace: namespace,
	}, nil
}

// listRunner uses action.List with optimizations to efficiently find the latest release
type listRunner struct {
	action    *action.List
	namespace string
}

func (l *listRunner) Run(ctx context.Context, name string) (*release.Release, error) {
	// Set filter for exact release name match
	l.action.Filter = "^" + regexp.QuoteMeta(name) + "$"

	// Run in goroutine since Helm SDK doesn't support context cancellation
	type result struct {
		rel *release.Release
		err error
	}
	resultChan := make(chan result, 1)

	go func() {
		releases, err := l.action.Run()
		if err != nil {
			resultChan <- result{err: err}
			return
		}
		if len(releases) == 0 {
			resultChan <- result{err: fmt.Errorf("release %q not found in namespace %q", name, l.namespace)}
			return
		}
		// Type assert from release.Releaser interface to concrete type
		resultChan <- result{rel: releases[0].(*release.Release)}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.rel, res.err
	}
}

// MockStatusRunner for testing
type MockStatusRunner struct {
	Release *release.Release
	Err     error
}

func (m *MockStatusRunner) Run(ctx context.Context, name string) (*release.Release, error) {
	return m.Release, m.Err
}

// MockHelmFactory for testing - allows injecting mock status runners
type MockHelmFactory struct {
	Runner StatusRunner
	Err    error
}

func (f *MockHelmFactory) GetStatusRunner(kubeContext, namespace string, log *slog.Logger) (StatusRunner, error) {
	return f.Runner, f.Err
}

// ClientFactory is the global factory - replaceable for testing
var ClientFactory HelmClientFactory = &DefaultHelmFactory{}
