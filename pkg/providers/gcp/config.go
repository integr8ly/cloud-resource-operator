package gcp

import (
	"context"
	"encoding/json"
	"github.com/integr8ly/cloud-resource-operator/internal/k8sutil"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	DefaultConfigMapName = "cloud-resources-gcp-strategies"
	defaultReconcileTime = time.Second * 30
	DefaultFinalizer     = "finalizers.cloud-resources-operator.integreatly.org"
)

//DefaultConfigMapNamespace is the default namespace that Configmaps will be created in
var DefaultConfigMapNamespace, _ = k8sutil.GetWatchNamespace()

type StrategyConfig struct {
	Region      string          `json:"region"`
	ProjectID   string          `json:"projectID"`
	RawStrategy json.RawMessage `json:"strategy"`
}

type ConfigManager interface {
	ReadStorageStrategy(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error)
}

type ConfigMapConfigManager struct {
	configMapName      string
	configMapNamespace string
	client             client.Client
}

func NewConfigMapConfigManager(cm string, namespace string, client client.Client) *ConfigMapConfigManager {
	if cm == "" {
		cm = DefaultConfigMapName
	}
	if namespace == "" {
		namespace = DefaultConfigMapNamespace
	}
	return &ConfigMapConfigManager{
		configMapName:      cm,
		configMapNamespace: namespace,
		client:             client,
	}
}

func NewDefaultConfigManager(client client.Client) *ConfigMapConfigManager {
	return NewConfigMapConfigManager(DefaultConfigMapName, DefaultConfigMapNamespace, client)
}

func (cm ConfigMapConfigManager) ReadStorageStrategy(ctx context.Context, rt providers.ResourceType, tier string) (*StrategyConfig, error) {
	//TODO implement me
	return nil, nil
}

var _ ConfigManager = (*ConfigMapConfigManager)(nil)
