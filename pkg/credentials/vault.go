package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/pkg/errors"
)

var TypeVault = "vault"

type Vault struct {
	ctl *vault.Client
	sb  SecretBus

	Engine string
	Mount  string

	KeyEntry
	Keys []KeyEntry
}

func (v *Vault) Name() string {
	return TypeVault
}

func (v *Vault) SetSecretBus(bus SecretBus, opts ...Option) {
	applyOpts(v, opts)
	v.sb = bus
}

func (v *Vault) ExposeSecret(sec Secret, opts ...Option) error {
	applyOpts(v, opts)

	// Collect all the keys
	keySet := utils.SetFrom[KeyEntry](append(v.Keys, v.KeyEntry)...)

	for _, p := range keySet.Items() {
		dest := strings.TrimSpace(utils.CoalesceZero[string](p.Target, p.Key, p.Path))

		if v.sb != nil {
			v.sb = context.WithValue(v.sb, dest, sec)
		}

		if v.Expose {
			if err := os.Setenv(dest, fmt.Sprint(sec)); err != nil {
				return errors.Wrap(err, "failed to set environment variable")
			}
		}
	}

	return nil
}

func (v *Vault) GetSecret(opts ...Option) (secrets []Secret, err error) {
	applyOpts(v, opts)

	if v.ctl == nil {
		// @TODO -> Support custom configuration
		v.ctl, err = vault.NewClient(nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create the Vault client")
		}
	}

	// Collect all the keys
	ctx := context.Background()

itemLoop:
	for _, p := range append([]KeyEntry{v.KeyEntry}, v.Keys...) {
		var cancel context.CancelFunc
		if p.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		}
		switch v.Engine {
		case "kv", "kv-v1", "kvv1":
			fetchedSec, vErr := v.ctl.KVv1(v.Mount).Get(ctx, p.Path)
			if vErr != nil {
				utils.CancelContext(cancel)
				return nil, errors.Wrap(vErr, "failed to get Vault kv-v1 secret")
			}
			if vErr != nil {
				utils.CancelContext(cancel)
				return nil, errors.Wrap(vErr, "failed to get Vault kv-v2 secret")
			}
			if fetchedSec == nil || fetchedSec.Data == nil {
				utils.CancelContext(cancel)
				continue itemLoop
			}
			utils.CancelContext(cancel)
			secrets = append(secrets, fetchedSec.Data[p.Key])
		case "kv2", "kv-v2", "kvv2":
			fetchedSec, vErr := v.ctl.KVv2(v.Mount).Get(ctx, p.Path)
			if vErr != nil {
				utils.CancelContext(cancel)
				return nil, errors.Wrap(vErr, "failed to get Vault kv-v2 secret")
			}
			if fetchedSec == nil || fetchedSec.Data == nil {
				utils.CancelContext(cancel)
				continue itemLoop
			}
			utils.CancelContext(cancel)
			secrets = append(secrets, fetchedSec.Data[p.Key])
		default:
			utils.CancelContext(cancel)
			return nil, errors.New("unsupported Vault engine")
		}
	}
	return secrets, nil
}
