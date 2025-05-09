//go:generate go run ./common/generator.go

package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/isometry/platform-health/pkg/controllers/k8s"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/provider"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/mcuadros/go-defaults"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const TypeKubernetes = "kubernetes"

type Kubernetes struct {
	Group     string        `mapstructure:"group" default:"apps"`
	Version   string        `mapstructure:"version" default:"v1"`
	Kind      string        `mapstructure:"kind" default:"deployment"`
	Namespace string        `mapstructure:"namespace" default:"default"`
	Name      string        `mapstructure:"name"`
	Condition *Condition    `mapstructure:"condition"`
	Timeout   time.Duration `mapstructure:"timeout" default:"10s"`
}

type Condition struct {
	Type   string `mapstructure:"type" default:"Available"`
	Status string `mapstructure:"status" default:"True"`
}

type GV struct {
	Group   string
	Version string
}

func init() {
	provider.Register(TypeKubernetes, new(Kubernetes))
}

func (i *Kubernetes) LogValue() slog.Value {
	logAttr := []slog.Attr{
		slog.String("group", i.Group),
		slog.String("version", i.Version),
		slog.String("kind", i.Kind),
		slog.String("name", i.Name),
		slog.String("namespace", i.Namespace),
		slog.Any("timeout", i.Timeout),
	}
	return slog.GroupValue(logAttr...)
}

func (i *Kubernetes) SetDefaults() {
	defaults.SetDefaults(i)
}

func (i *Kubernetes) GetType() string {
	return TypeKubernetes
}

func (i *Kubernetes) GetName() string {
	return fmt.Sprintf("%s/%s", i.Kind, i.Name)
}

func (i *Kubernetes) GetHealth(ctx context.Context) *ph.HealthCheckResponse {
	log := utils.ContextLogger(ctx, slog.String("provider", TypeKubernetes), slog.Any("instance", i))
	log.Debug("checking")

	component := &ph.HealthCheckResponse{
		Type: TypeKubernetes,
		Name: i.GetName(),
	}
	defer component.LogStatus(log)

	k8sController, err := k8s.NewController(k8s.WithTimeout(i.Timeout))
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	// fix default group and version for common resources
	if i.Group == "apps" && i.Version == "v1" && i.Kind != "deployment" {
		k := strings.ToLower(i.Kind)
		if gv, ok := commonKindToGV[k]; ok {
			i.Group = gv.Group
			i.Version = gv.Version
		}
	}

	gvk := schema.GroupVersionKind{
		Group:   i.Group,
		Version: i.Version,
		Kind:    i.Kind,
	}

	blob, err := k8sController.GetResource(ctx, i.Namespace, i.Name, gvk, metav1.GetOptions{})
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	resource, err := NewResource(blob.Object)
	if err != nil {
		return component.Unhealthy(err.Error())
	}

	if i.Condition != nil {
		for _, condition := range resource.Status.Conditions {
			if string(condition.Type) == i.Condition.Type {
				if string(condition.Status) == i.Condition.Status {
					return component.Healthy()
				} else {
					return component.Unhealthy(fmt.Sprintf("condition %s is %s", i.Condition.Type, condition.Status))
				}
			}
		}
	}

	return component.Healthy()
}
