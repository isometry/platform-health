package testutil

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DeploymentBuilder creates test Deployment resources.
type DeploymentBuilder struct {
	name, namespace string
	replicas        int64
	ready           bool
	labels          map[string]string
	generation      int64
}

// NewDeployment creates a new DeploymentBuilder.
func NewDeployment(name, namespace string) *DeploymentBuilder {
	return &DeploymentBuilder{
		name:       name,
		namespace:  namespace,
		replicas:   3,
		ready:      true,
		labels:     map[string]string{"app": name},
		generation: 1,
	}
}

// WithReplicas sets the number of replicas.
func (b *DeploymentBuilder) WithReplicas(n int64) *DeploymentBuilder {
	b.replicas = n
	return b
}

// Unhealthy marks the deployment as not ready.
func (b *DeploymentBuilder) Unhealthy() *DeploymentBuilder {
	b.ready = false
	return b
}

// WithLabels sets the labels.
func (b *DeploymentBuilder) WithLabels(labels map[string]string) *DeploymentBuilder {
	b.labels = labels
	return b
}

// Build creates the Deployment unstructured object.
func (b *DeploymentBuilder) Build() *unstructured.Unstructured {
	availableStatus := "True"
	progressingStatus := "True"
	availableReplicas, updatedReplicas := b.replicas, b.replicas
	if !b.ready {
		availableStatus = "False"
		progressingStatus = "False"
		availableReplicas = 0
		updatedReplicas = 0
	}

	labelsAny := make(map[string]any)
	for k, v := range b.labels {
		labelsAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":       b.name,
				"namespace":  b.namespace,
				"generation": b.generation,
				"labels":     labelsAny,
			},
			"spec": map[string]any{
				"replicas": b.replicas,
			},
			"status": map[string]any{
				"observedGeneration": b.generation,
				"replicas":           b.replicas,
				"readyReplicas":      availableReplicas,
				"availableReplicas":  availableReplicas,
				"updatedReplicas":    updatedReplicas,
				"conditions": []any{
					map[string]any{
						"type":   "Available",
						"status": availableStatus,
						"reason": "MinimumReplicasAvailable",
					},
					map[string]any{
						"type":   "Progressing",
						"status": progressingStatus,
						"reason": "NewReplicaSetAvailable",
					},
				},
			},
		},
	}
}

// ConfigMapBuilder creates test ConfigMap resources.
type ConfigMapBuilder struct {
	name, namespace string
	data            map[string]string
	labels          map[string]string
}

// NewConfigMap creates a new ConfigMapBuilder.
func NewConfigMap(name, namespace string) *ConfigMapBuilder {
	return &ConfigMapBuilder{
		name:      name,
		namespace: namespace,
		data:      make(map[string]string),
		labels:    make(map[string]string),
	}
}

// WithData sets the ConfigMap data.
func (b *ConfigMapBuilder) WithData(data map[string]string) *ConfigMapBuilder {
	b.data = data
	return b
}

// WithLabels sets the labels.
func (b *ConfigMapBuilder) WithLabels(labels map[string]string) *ConfigMapBuilder {
	b.labels = labels
	return b
}

// Build creates the ConfigMap unstructured object.
func (b *ConfigMapBuilder) Build() *unstructured.Unstructured {
	dataAny := make(map[string]any)
	for k, v := range b.data {
		dataAny[k] = v
	}
	labelsAny := make(map[string]any)
	for k, v := range b.labels {
		labelsAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      b.name,
				"namespace": b.namespace,
				"labels":    labelsAny,
			},
			"data": dataAny,
		},
	}
}

// NamespaceBuilder creates test Namespace resources.
type NamespaceBuilder struct {
	name   string
	labels map[string]string
	phase  string
}

// NewNamespace creates a new NamespaceBuilder.
func NewNamespace(name string) *NamespaceBuilder {
	return &NamespaceBuilder{
		name:   name,
		labels: make(map[string]string),
		phase:  "Active",
	}
}

// WithLabels sets the labels.
func (b *NamespaceBuilder) WithLabels(labels map[string]string) *NamespaceBuilder {
	b.labels = labels
	return b
}

// Terminating marks the namespace as terminating.
func (b *NamespaceBuilder) Terminating() *NamespaceBuilder {
	b.phase = "Terminating"
	return b
}

// Build creates the Namespace unstructured object.
func (b *NamespaceBuilder) Build() *unstructured.Unstructured {
	labelsAny := make(map[string]any)
	for k, v := range b.labels {
		labelsAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":   b.name,
				"labels": labelsAny,
			},
			"status": map[string]any{
				"phase": b.phase,
			},
		},
	}
}

// PodDisruptionBudgetBuilder creates test PDB resources.
type PodDisruptionBudgetBuilder struct {
	name, namespace string
	minAvailable    int64
	selector        map[string]string
}

// NewPodDisruptionBudget creates a new PodDisruptionBudgetBuilder.
func NewPodDisruptionBudget(name, namespace string) *PodDisruptionBudgetBuilder {
	return &PodDisruptionBudgetBuilder{
		name:         name,
		namespace:    namespace,
		minAvailable: 1,
		selector:     map[string]string{"app": name},
	}
}

// WithMinAvailable sets the minAvailable field.
func (b *PodDisruptionBudgetBuilder) WithMinAvailable(n int64) *PodDisruptionBudgetBuilder {
	b.minAvailable = n
	return b
}

// WithSelector sets the selector labels.
func (b *PodDisruptionBudgetBuilder) WithSelector(labels map[string]string) *PodDisruptionBudgetBuilder {
	b.selector = labels
	return b
}

// Build creates the PDB unstructured object.
func (b *PodDisruptionBudgetBuilder) Build() *unstructured.Unstructured {
	selectorAny := make(map[string]any)
	for k, v := range b.selector {
		selectorAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]any{
				"name":      b.name,
				"namespace": b.namespace,
			},
			"spec": map[string]any{
				"minAvailable": b.minAvailable,
				"selector": map[string]any{
					"matchLabels": selectorAny,
				},
			},
		},
	}
}

// PodBuilder creates test Pod resources.
type PodBuilder struct {
	name, namespace string
	labels          map[string]string
	phase           string
}

// NewPod creates a new PodBuilder.
func NewPod(name, namespace string) *PodBuilder {
	return &PodBuilder{
		name:      name,
		namespace: namespace,
		labels:    map[string]string{"app": name},
		phase:     "Running",
	}
}

// WithLabels sets the labels.
func (b *PodBuilder) WithLabels(labels map[string]string) *PodBuilder {
	b.labels = labels
	return b
}

// WithPhase sets the pod phase.
func (b *PodBuilder) WithPhase(phase string) *PodBuilder {
	b.phase = phase
	return b
}

// Build creates the Pod unstructured object.
func (b *PodBuilder) Build() *unstructured.Unstructured {
	labelsAny := make(map[string]any)
	for k, v := range b.labels {
		labelsAny[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      b.name,
				"namespace": b.namespace,
				"labels":    labelsAny,
			},
			"status": map[string]any{
				"phase": b.phase,
			},
		},
	}
}
