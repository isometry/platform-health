package helm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mcuadros/go-defaults"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"

	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
)

const TypeHelm = "helm"

type Helm struct {
	Name      string        `mapstructure:"name"`
	Chart     string        `mapstructure:"chart"`
	Namespace string        `mapstructure:"namespace"`
	Timeout   time.Duration `mapstructure:"timeout" default:"5s"`
}

func init() {
	provider.Register(TypeHelm, new(Helm))
}

func (i *Helm) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("name", i.Name),
		slog.String("chart", i.Chart),
		slog.String("namespace", i.Namespace),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Helm) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *Helm) GetType() string {
	return TypeHelm
}

func (i *Helm) GetName() string {
	return i.Name
}

func (i *Helm) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeHelm), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeHelm,
		Name: i.Name,
	}
	defer component.LogStatus(log)

	client := cli.New()
	client.SetNamespace(i.Namespace)
	clientgetter := client.RESTClientGetter()
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(clientgetter, i.Namespace, "secret", func(format string, v ...any) { log.Debug(fmt.Sprintf(format, v...)) }); err != nil {
		return component.Unhealthy(err.Error())
	}

	statusAction := action.NewStatus(actionConfig)

	resultChan := make(chan error)
	go func() {
		status, err := statusAction.Run(i.Name)
		if err != nil {
			resultChan <- err
			return
		}
		if status.Info.Status != "deployed" {
			resultChan <- fmt.Errorf("expected status 'deployed'; actual status '%s'", status.Info.Status)
			return
		}
		resultChan <- nil
	}()

	select {
	case <-time.After(i.Timeout):
		return component.Unhealthy("timeout")
	case err := <-resultChan:
		if err != nil {
			return component.Unhealthy(err.Error())
		}
	}

	return component.Healthy()
}
