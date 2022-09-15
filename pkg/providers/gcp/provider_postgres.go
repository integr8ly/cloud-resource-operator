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

const postgresProviderName = "gcp-cloudsql"

type PostgresProvider struct {
	Client            client.Client
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewGCPPostgresProvider(client client.Client) *PostgresProvider {
	return &PostgresProvider{
		Client:            client,
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (pp PostgresProvider) GetName() string {
	return postgresProviderName
}

func (pp PostgresProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (pp PostgresProvider) GetReconcileTime(p *v1alpha1.Postgres) time.Duration {
	if p.Status.Phase != types.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (pp PostgresProvider) ReconcilePostgres(ctx context.Context, p *v1alpha1.Postgres) (*providers.PostgresInstance, types.StatusMessage, error) {
	_, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return nil, types.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}
	// TODO implement me
	return nil, "", nil
}

func (pp PostgresProvider) DeletePostgres(ctx context.Context, p *v1alpha1.Postgres) (types.StatusMessage, error) {
	_, err := pp.CredentialManager.ReconcileProviderCredentials(ctx, p.Namespace)
	if err != nil {
		errMsg := fmt.Sprintf("failed to reconcile gcp postgres provider credentials for postgres instance %s", p.Name)
		return types.StatusMessage(errMsg), fmt.Errorf("%s: %w", errMsg, err)
	}
	// TODO implement me
	return "", nil
}

var _ providers.PostgresProvider = (*PostgresProvider)(nil)
