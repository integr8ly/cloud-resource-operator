package openshift

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OpenShiftPostgresDeploymentDetails struct {
	Connection map[string][]byte
}

func (d *OpenShiftPostgresDeploymentDetails) Data() map[string][]byte {
	return d.Connection
}

type OpenShiftPostgresProvider struct {
	Client        client.Client
	Logger        *logrus.Entry
	ConfigManager ConfigManager
}

func NewOpenShiftPostgresProvider(client client.Client, logger *logrus.Entry) *OpenShiftPostgresProvider {
	return &OpenShiftPostgresProvider{
		Client:        client,
		Logger:        logger.WithFields(logrus.Fields{"provider": "openshift_postgres"}),
		ConfigManager: NewDefaultConfigManager(client),
	}
}

func (p *OpenShiftPostgresProvider) GetName() string {
	return providers.OpenShiftDeploymentStrategy
}

func (p *OpenShiftPostgresProvider) SupportsStrategy(d string) bool {
	return d == providers.OpenShiftDeploymentStrategy
}

func (p *OpenShiftPostgresProvider) CreatePostgres(ctx context.Context, ps *v1alpha1.Postgres) (*providers.PostgresInstance, error) {
	return nil, nil
}

func (p *OpenShiftPostgresProvider) DeletePostgres(ctx context.Context, ps *v1alpha1.Postgres) error {
	return nil
}
