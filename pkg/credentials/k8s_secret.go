package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/isometry/platform-health/pkg/controllers/k8s"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var TypeK8sSecret = "k8s_secret"

type K8sEntry struct {
	KeyEntry
	Namespace string
}

type K8sSecret struct {
	ctl *k8s.Controller
	sb  SecretBus

	K8sEntry
	Keys []K8sEntry
}

func (k *K8sSecret) Name() string {
	return TypeK8sSecret
}

func (k *K8sSecret) SetSecretBus(bus SecretBus, opts ...Option) {
	applyOpts(k, opts)
	k.sb = bus
}

func (k *K8sSecret) ExposeSecret(sec Secret, opts ...Option) error {
	applyOpts(k, opts)

	// Collect all the keys
	keySet := utils.SetFrom[K8sEntry](append(k.Keys, k.K8sEntry)...)
	for _, p := range keySet.Items() {
		dest := strings.TrimSpace(utils.CoalesceZero[string](p.Target, p.Key, p.Path))

		if k.sb != nil {
			k.sb = context.WithValue(k.sb, dest, sec)
		}

		if p.Expose {
			if err := os.Setenv(dest, fmt.Sprint(sec)); err != nil {
				return errors.Wrap(err, "failed to set environment variable")
			}
		}
	}

	return nil
}

func (k *K8sSecret) GetSecret(opts ...Option) (secrets []Secret, err error) {
	applyOpts(k, opts)

	if k.ctl == nil {
		// @TODO -> support custom configuration
		k.ctl, err = k8s.NewController()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create AWS controller")
		}
	}

	// Collect all the keys
	ctx := context.Background()
	for _, p := range append([]K8sEntry{k.K8sEntry}, k.Keys...) {
		var cancel context.CancelFunc
		if p.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		}

		var secret any
		k8sSecret, err := k.ctl.GetSecret(ctx, p.Namespace, p.Path, metav1.GetOptions{})
		if err != nil {
			utils.CancelContext(cancel)
			return nil, errors.Wrap(err, "failed to get K8s secret")
		}
		if p.Key == "" {
			secret, err = k8sSecret.Data[p.Path], nil
		}

		if err != nil {
			utils.CancelContext(cancel)
			return nil, errors.Wrap(err, "failed to get AWS Secrets credentials secret")
		}
		utils.CancelContext(cancel)

		if p.Key != "" {

		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}
