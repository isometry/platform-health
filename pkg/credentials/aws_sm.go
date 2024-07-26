package credentials

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/isometry/platform-health/pkg/controllers/aws"
	"github.com/isometry/platform-health/pkg/utils"
	"github.com/pkg/errors"
)

var TypeSM = "aws_sm"

type SM struct {
	ctl *aws.Controller
	sb  SecretBus

	KeyEntry
	Keys []KeyEntry
}

func (s *SM) Name() string {
	return TypeSM
}

func (s *SM) SetSecretBus(bus SecretBus, opts ...Option) {
	applyOpts(s, opts)
	s.sb = bus
}

func (s *SM) ExposeSecret(sec Secret, opts ...Option) error {
	applyOpts(s, opts)

	// Collect all the keys
	for _, p := range append([]KeyEntry{s.KeyEntry}, s.Keys...) {
		dest := strings.TrimSpace(utils.CoalesceZero[string](p.Target, p.Key, p.Path))

		if s.sb != nil {
			s.sb = context.WithValue(s.sb, dest, sec)
		}

		if p.Expose {
			if err := os.Setenv(dest, fmt.Sprint(sec)); err != nil {
				return errors.Wrap(err, "failed to set environment variable")
			}
		}
	}

	return nil
}

func (s *SM) GetSecret(opts ...Option) (secrets []Secret, err error) {
	applyOpts(s, opts)

	if s.ctl == nil {
		// @TODO -> support custom configuration
		s.ctl, err = aws.NewController()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create AWS controller")
		}
	}

	// Collect all the keys
	keySet := utils.SetFrom[KeyEntry](append(s.Keys, s.KeyEntry)...)

	ctx := context.Background()
	for _, p := range keySet.Items() {
		var cancel context.CancelFunc
		if p.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		}

		var secret any
		if p.Key == "" {
			secret, err = s.ctl.GetSecretManagerSecret(p.Path)
		} else {
			secret, err = s.ctl.GetSecretManagerSecretKey(p.Path, p.Key)
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
