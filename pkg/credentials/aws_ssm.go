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

var TypeSSM = "aws_ssm"

type SSMKeyEntry struct {
	KeyEntry
	Decryption bool
}

type SSM struct {
	ctl *aws.Controller
	sb  SecretBus

	SSMKeyEntry
	Keys       []SSMKeyEntry
	Decryption bool
}

func (s *SSM) Name() string {
	return TypeSSM
}

func (s *SSM) SetSecretBus(bus SecretBus, opts ...Option) {
	applyOpts(s, opts)
	s.sb = bus
}

func (s *SSM) ExposeSecret(sec Secret, opts ...Option) error {
	applyOpts(s, opts)

	// Collect all the keys
	keySet := utils.SetFrom[SSMKeyEntry](append(s.Keys, s.SSMKeyEntry)...)
	for _, p := range keySet.Items() {
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

func (s *SSM) GetSecret(opts ...Option) (secrets []Secret, err error) {
	applyOpts(s, opts)

	if s.ctl == nil {
		// @TODO -> support custom configuration
		s.ctl, err = aws.NewController()
		if err != nil {
			return nil, errors.Wrap(err, "failed to create AWS controller")
		}
	}

	// Collect all the keys
	ctx := context.Background()
	for _, p := range append([]SSMKeyEntry{s.SSMKeyEntry}, s.Keys...) {
		var cancel context.CancelFunc
		if p.Timeout > 0 {
			ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		}
		decryption := s.Decryption
		if p.Decryption {
			decryption = p.Decryption
		}
		var secret any
		if p.Key == "" {
			secret, err = s.ctl.GetSystemsManagerSecret(p.Path, decryption)
		} else {
			secret, err = s.ctl.GetSystemsManagerSecretKey(p.Path, p.Key, decryption)
		}
		if err != nil {
			utils.CancelContext(cancel)
			return nil, errors.Wrap(err, "failed to get AWS SSM secret")
		}
		utils.CancelContext(cancel)
		secrets = append(secrets, secret)
	}

	return secrets, nil
}
