package resources

import (
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelClusterIDKey   = "clusterID"
	LabelResourceIDKey  = "resourceID"
	LabelNamespaceKey   = "namespace"
	LabelInstanceIDKey  = "instanceID"
	LabelProductNameKey = "productName"
	LabelStrategyKey    = "strategy"
	LabelStatusKey      = "status"
	LabelStatusPhaseKey = "statusPhase"
)

func GetGenericMetricLabelNames() []string {
	return []string{
		LabelClusterIDKey,
		LabelResourceIDKey,
		LabelNamespaceKey,
		LabelInstanceIDKey,
		LabelProductNameKey,
		LabelStrategyKey,
	}
}

// BuildGenericMetricLabels returns generic labels to be added to every metric
func BuildGenericMetricLabels(r v1.ObjectMeta, clusterID, cacheName, providerName string) map[string]string {
	return map[string]string{
		LabelClusterIDKey:   clusterID,
		LabelResourceIDKey:  r.Name,
		LabelNamespaceKey:   r.Namespace,
		LabelInstanceIDKey:  cacheName,
		LabelProductNameKey: r.Labels["productName"],
		LabelStrategyKey:    providerName,
	}
}

// BuildInfoMetricLabels adds extra information to labels around resource
func BuildInfoMetricLabels(r v1.ObjectMeta, status string, clusterID, cacheName, providerName string) map[string]string {
	labels := BuildGenericMetricLabels(r, clusterID, cacheName, providerName)
	if status != "" {
		labels[LabelStatusKey] = status
		return labels
	}
	labels[LabelStatusKey] = "nil"
	return labels
}

func BuildStatusMetricsLabels(r v1.ObjectMeta, clusterID, cacheName, providerName string, phase croType.StatusPhase) map[string]string {
	labels := BuildGenericMetricLabels(r, clusterID, cacheName, providerName)
	labels[LabelStatusPhaseKey] = string(phase)
	return labels
}
