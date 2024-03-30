package kubernetes

import (
	"fmt"

	"github.com/mitchellh/mapstructure"
	v1 "k8s.io/api/core/v1"
)

type Resource struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Conditions []struct {
			Type    string             `json:"type"`
			Status  v1.ConditionStatus `json:"status"`
			Message string             `json:"message,omitempty"`
		} `json:"conditions"`
	} `json:"status"`
}

func NewResource(obj any) (resource Resource, err error) {
	if obj == nil {
		return Resource{}, fmt.Errorf("object is nil")
	}

	if err := mapstructure.Decode(obj, &resource); err != nil {
		return Resource{}, err
	}

	return resource, nil
}
