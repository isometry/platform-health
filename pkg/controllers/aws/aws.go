package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/logging"
	"github.com/pkg/errors"
)

type Controller struct {
	ctx    context.Context
	logger *slog.Logger

	config    *aws.Config
	ssmClient *ssm.Client
	smClient  *secretsmanager.Client
}

type Option func(*Controller)

func NewController(opts ...Option) (*Controller, error) {
	_inst := &Controller{}
	for _, opt := range opts {
		opt(_inst)
	}
	if _inst.logger == nil {
		_inst.logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	_inst.logger = _inst.logger.With("controller", "AWSController")
	if _inst.ctx == nil {
		_inst.ctx = context.Background()
	}
	if _inst.config == nil {
		_inst.logger.Debug("loading default AWSController configuration...")
		cfg, err := config.LoadDefaultConfig(_inst.ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load AWSController configuration")
		}
		cfg.Logger = newAWSLogger(_inst.logger)
		_inst.config = &cfg
	}

	_inst.ssmClient = ssm.NewFromConfig(*_inst.config)
	_inst.smClient = secretsmanager.NewFromConfig(*_inst.config)
	return _inst, nil
}

func (a *Controller) GetSystemsManagerSecret(path string, encrypted bool) (*string, error) {
	a.logger.With("path", path).Debug("fetching SSM secret...")
	ssmResponse, err := a.ssmClient.GetParameter(a.ctx, &ssm.GetParameterInput{
		Name:           aws.String(path),
		WithDecryption: aws.Bool(encrypted),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to load SSM parameters")
	}
	return ssmResponse.Parameter.Value, nil
}

func (a *Controller) GetSystemsManagerSecretKey(path, key string, encrypted bool) (any, error) {
	a.logger.With("path", path, "key", key, "encrypted", encrypted).Debug("fetching SSM secret key...")
	ssmResponse, err := a.GetSystemsManagerSecret(path, encrypted)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err = json.Unmarshal([]byte(*ssmResponse), &raw); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal SSM secret")
	}
	if raw == nil || len(raw) == 0 {
		return nil, errors.New("empty secret")
	}
	return raw[key], nil
}

func (a *Controller) GetSecretManagerSecret(path string) (*string, error) {
	a.logger.With("path", path).Debug("fetching Secrets Manager secret...")
	smResponse, err := a.smClient.GetSecretValue(a.ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(path),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to load Secrets Manager secret")
	}
	return smResponse.SecretString, nil
}

func (a *Controller) GetSecretManagerSecretKey(path, key string) (any, error) {
	a.logger.With("path", path).With("key", key).Debug("fetching Secrets Manager secret key...")
	smResponse, err := a.GetSecretManagerSecret(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err = json.Unmarshal([]byte(*smResponse), &raw); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal Secret Manager secret")
	}

	if raw == nil || len(raw) == 0 {
		return nil, errors.New("empty secret")
	}

	return raw[key], nil
}

type awsLogger struct {
	logger *slog.Logger
}

func newAWSLogger(logger *slog.Logger) *awsLogger {
	return &awsLogger{logger}
}
func (a *awsLogger) Logf(classification logging.Classification, format string, args ...any) {
	a.logger.Debug(fmt.Sprintf("[%v] %s", classification, fmt.Sprintf(format, args...)))
}
