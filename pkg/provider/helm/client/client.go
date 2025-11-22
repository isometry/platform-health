package client

import (
	"log/slog"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	k8sclient "github.com/isometry/platform-health/pkg/provider/kubernetes/client"
)

// StatusDeployed is exported for use in tests and provider
const StatusDeployed = common.StatusDeployed

// StatusRunner abstracts the helm status action for testing
type StatusRunner interface {
	Run(name string) (*release.Release, error)
}

// HelmClientFactory creates helm status runners
type HelmClientFactory interface {
	GetStatusRunner(namespace string, log *slog.Logger) (StatusRunner, error)
}

// DefaultHelmFactory creates real helm clients using kubernetes config
type DefaultHelmFactory struct{}

func (f *DefaultHelmFactory) GetStatusRunner(namespace string, log *slog.Logger) (StatusRunner, error) {
	config, err := k8sclient.GetKubeConfig()
	if err != nil {
		return nil, err
	}

	// Create ConfigFlags from rest.Config
	kubeConfig := genericclioptions.NewConfigFlags(false)
	kubeConfig.APIServer = &config.Host
	kubeConfig.BearerToken = &config.BearerToken
	kubeConfig.CAFile = &config.CAFile
	kubeConfig.Namespace = &namespace

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(kubeConfig, namespace, "secret"); err != nil {
		return nil, err
	}

	return &statusRunner{
		action: action.NewStatus(actionConfig),
	}, nil
}

// statusRunner wraps action.Status to return concrete *release.Release
type statusRunner struct {
	action *action.Status
}

func (s *statusRunner) Run(name string) (*release.Release, error) {
	releaser, err := s.action.Run(name)
	if err != nil {
		return nil, err
	}
	// Type assert from ri.Releaser interface to concrete type
	return releaser.(*release.Release), nil
}

// MockStatusRunner for testing
type MockStatusRunner struct {
	Release *release.Release
	Err     error
}

func (m *MockStatusRunner) Run(name string) (*release.Release, error) {
	return m.Release, m.Err
}

// MockHelmFactory for testing - allows injecting mock status runners
type MockHelmFactory struct {
	Runner StatusRunner
	Err    error
}

func (f *MockHelmFactory) GetStatusRunner(namespace string, log *slog.Logger) (StatusRunner, error) {
	return f.Runner, f.Err
}

// ClientFactory is the global factory - replaceable for testing
var ClientFactory HelmClientFactory = &DefaultHelmFactory{}
