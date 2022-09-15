package gcp

import (
	"context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const redisProviderName = "gcp-memorystore"

type RedisProvider struct {
	Client            client.Client
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewGCPRedisProvider(client client.Client) *RedisProvider {
	return &RedisProvider{
		Client:            client,
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (rp RedisProvider) GetName() string {
	return redisProviderName
}

func (rp RedisProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (rp RedisProvider) GetReconcileTime(r *v1alpha1.Redis) time.Duration {
	if r.Status.Phase != types.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (rp RedisProvider) CreateRedis(ctx context.Context, r *v1alpha1.Redis) (*providers.RedisCluster, types.StatusMessage, error) {
	_, err := rp.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return nil, types.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}
	//TODO implement me
	return nil, "", nil
}

func (rp RedisProvider) DeleteRedis(ctx context.Context, r *v1alpha1.Redis) (types.StatusMessage, error) {
	_, err := rp.CredentialManager.ReconcileProviderCredentials(ctx, r.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp redis provider credentials for redis instance %s", r.Name)
		return types.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}
	//TODO implement me
	return "", nil
}

var _ providers.RedisProvider = (*RedisProvider)(nil)
