package providers

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/types"

	errorUtil "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultConfigNamespace       = "kube-system"
	DefaultProviderConfigMapName = "cloud-resource-config"
)

type DeploymentStrategyMapping struct {
	BlobStorage string `json:"blobstorage"`
}

type ConfigManager struct {
	client                     client.Client
	providerConfigMapName      string
	providerConfigMapNamespace string
}

func NewConfigManager(cm string, namespace string, client client.Client) *ConfigManager {
	if cm == "" {
		cm = DefaultProviderConfigMapName
	}
	if namespace == "" {
		namespace = DefaultConfigNamespace
	}
	return &ConfigManager{
		client:                     client,
		providerConfigMapName:      cm,
		providerConfigMapNamespace: namespace,
	}
}

// Get high-level information about the strategy used in a deployment type
func (m *ConfigManager) GetStrategyMappingForDeploymentType(ctx context.Context, t string) (*DeploymentStrategyMapping, error) {
	cm := &v1.ConfigMap{}
	err := m.client.Get(ctx, types.NamespacedName{Name: m.providerConfigMapName, Namespace: m.providerConfigMapNamespace}, cm)
	if err != nil {
		return nil, errorUtil.Wrapf(err, "failed to read provider config from configmap %s in namespace %s", m.providerConfigMapName, m.providerConfigMapNamespace)
	}
	dsm := &DeploymentStrategyMapping{}
	if err = json.Unmarshal([]byte(cm.Data[t]), dsm); err != nil {
		return nil, errorUtil.Wrapf(err, "failed to unmarshal config for deployment type %s", t)
	}
	return dsm, nil
}
