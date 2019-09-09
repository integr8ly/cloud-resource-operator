package openshift

import (
	"context"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenShiftRedisProvider struct {
	Client client.Client
}

func NewOpenShiftRedisProvider(client client.Client) *OpenShiftRedisProvider {
	return &OpenShiftRedisProvider{Client: client}
}

func (p *OpenShiftRedisProvider) GetName() string {
	return providers.OpenShiftDeploymentStrategy
}

func (p *OpenShiftRedisProvider) SupportsStrategy(d string) bool {
	return d == providers.OpenShiftDeploymentStrategy
}

func (p *OpenShiftRedisProvider) CreateRedis(ctx context.Context) error {
	return nil
}

func (p *OpenShiftRedisProvider) DeleteRedis(ctx context.Context) error {
	return nil
}
