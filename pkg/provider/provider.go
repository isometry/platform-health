package provider

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/isometry/platform-health/pkg/controllers/k8s"
	ph "github.com/isometry/platform-health/pkg/platform_health"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/durationpb"
	v1core "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// refTag is the tag used to specify when a field should be populated by fetching a Kubernetes object reference
	refTag = "ref"
	// refTagFormat is the format used to specify the Kubernetes object references (objectRef:dataKey)
	refTagFormat = regexp.MustCompile("^(?P<objectRef>[^:]+):(?P<dataKey>.+)$")
)

// Instance is the interface that must be implemented by all providers.
type Instance interface {
	// GetType returns the provider type of the instance
	GetType() string
	// GetName returns the name of the instance
	GetName() string
	// GetHealth checks and returns the instance
	GetHealth(context.Context) *ph.HealthCheckResponse
	// SetDefaults sets the default values for the instance
	SetDefaults()
}

// Config is the interface through which the provider configuration is retrieved.
type Config interface {
	GetInstances() []Instance
}

func Check(ctx context.Context, instances []Instance) (response []*ph.HealthCheckResponse, status ph.Status) {
	var wg sync.WaitGroup
	instanceChan := make(chan *ph.HealthCheckResponse, len(instances))

	for _, instance := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			instanceChan <- GetHealthWithDuration(ctx, instance)
		}()
	}

	go func() {
		wg.Wait()
		close(instanceChan)
	}()

	response = make([]*ph.HealthCheckResponse, 0, len(instances))
	status = ph.Status_HEALTHY
	for instance := range instanceChan {
		response = append(response, instance)

		if instance.Status.Number() > status.Number() {
			status = instance.Status
		}
	}

	return response, status
}

func GetHealthWithDuration(ctx context.Context, instance Instance) *ph.HealthCheckResponse {
	start := time.Now()
	response := instance.GetHealth(ctx)
	if response != nil {
		response.Duration = durationpb.New(time.Since(start))
	}
	return response
}

var _k8sController *k8s.Controller
var _refCache = make(map[string]*v1core.ConfigMap)

func init() {
	ctl, err := k8s.NewController()
	if err == nil {
		_k8sController = ctl
	}
}

// PopulateValuesWithRef populates the fields of the instance with the values of the Kubernetes object references.
func PopulateValuesWithRef(instance Instance) error {
	// Is deactivated
	if _k8sController == nil {
		return nil
	}

	values := utils.GetRefTagValueIfZero(refTag, instance)
	for key, value := range values {
		matches := refTagFormat.FindStringSubmatch(value)
		if len(matches) != 3 {
			return errors.Errorf("invalid ref tag value %s", value)
		}
		objectRef := matches[1]
		dataKey := matches[2]

		// Avoid repeated queries to the same object reference
		var cm *v1core.ConfigMap
		cachedCm, ok := _refCache[objectRef]
		if !ok {
			cm = cachedCm
		} else {
			fetched, err := _k8sController.GetConfigMap(context.Background(), "", objectRef, v1.GetOptions{})
			if err != nil {
				return errors.Wrapf(err, "failed to get kubernetes object reference %s", objectRef)
			}
			cm = fetched
		}

		dataValue, ok := cm.Data[dataKey]
		if !ok {
			return errors.Errorf("key %s not found in kubernetes reference %s", dataKey, objectRef)
		}

		if err := utils.SetField(instance, key, dataValue); err != nil {
			return errors.Wrapf(err, "failed to set field %s for instance %T", key, instance)
		}
	}
	return nil
}
