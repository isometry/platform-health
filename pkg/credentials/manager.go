package credentials

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	supportedCredentialsType = []string{
		TypeSM, TypeSSM, TypeVault, TypeK8sCM, TypeK8sSecret,
	}
)

type Params = map[string]any
type Entry = map[string]Params
type SecretBus context.Context

type KeyEntry struct {
	Path, Key, Target string
	Timeout           time.Duration
	Expose            bool
}

type manager struct {
	credentials []Entry
}

func GetSecretBus(credentials []Entry) (SecretBus, error) {
	if credentials == nil || len(credentials) == 0 {
		return context.TODO(), nil
	}
	_inst := manager{
		credentials: credentials,
	}
	return _inst.Authenticate()
}

func (m *manager) Authenticate() (SecretBus, error) {
	secretBus := context.TODO()
	for _, entry := range m.credentials {
		for ctl, params := range entry {
			creds, err := identifyController(ctl, params)
			if err != nil {
				return nil, err
			}
			// Support daisy-chaining of secrets
			secret, err := creds.GetSecret(
				WithSecretBus(secretBus))
			if err != nil {
				return nil, err
			}
			if err = creds.ExposeSecret(secret,
				WithSecretBus(secretBus)); err != nil {
				return nil, err
			}
		}
	}
	return secretBus, nil
}

func identifyController(ctl string, params Params) (Credentials, error) {
	credsType := strings.ToLower(ctl)
	if accepted := slices.Contains(supportedCredentialsType, credsType); !accepted {
		return nil, fmt.Errorf("unsupported controller: %s", ctl)
	}

	var creds Credentials
	switch credsType {
	case TypeSSM:
		creds = new(SSM)
	case TypeSM:
		creds = new(SM)
	case TypeVault:
		creds = new(Vault)
	case TypeK8sCM:
		creds = new(K8sCM)
	case TypeK8sSecret:
		creds = new(K8sSecret)
	}

	if err := mapToStructFields(creds, params); err != nil {
		return nil, err
	}
	return creds, nil
}
