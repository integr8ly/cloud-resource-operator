package aws

import (
	"context"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AWSRedisProvider struct {
	Client client.Client
}

func NewAWSRedisProvider(client client.Client) *AWSRedisProvider {
	return &AWSRedisProvider{Client: client}
}

func (p *AWSRedisProvider) GetName() string {
	return providers.AWSDeploymentStrategy
}

func (p *AWSRedisProvider) SupportsStrategy(d string) bool {
	return d == providers.AWSDeploymentStrategy
}

func (p *AWSRedisProvider) CreateRedis(ctx context.Context) error {
	return nil
}

func (p *AWSRedisProvider) DeleteRedis(ctx context.Context) error {
	return nil
}
