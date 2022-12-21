package resources

import (
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
)

// BuildRedisGenericMetricLabels returns generic labels to be added to every metric
func BuildRedisGenericMetricLabels(r *v1alpha1.Redis, clusterID, cacheName, providerName string) map[string]string {
	return map[string]string{
		"clusterID":   clusterID,
		"resourceID":  r.Name,
		"namespace":   r.Namespace,
		"instanceID":  cacheName,
		"productName": r.Labels["productName"],
		"strategy":    providerName,
	}
}

// BuildRedisInfoMetricLabels adds extra information to labels around resource
func BuildRedisInfoMetricLabels(r *v1alpha1.Redis, status string, clusterID, cacheName, providerName string) map[string]string {
	labels := BuildRedisGenericMetricLabels(r, clusterID, cacheName, providerName)
	if status != "" {
		labels["status"] = status
		return labels
	}
	labels["status"] = "nil"
	return labels
}

func BuildRedisStatusMetricsLabels(r *v1alpha1.Redis, clusterID, cacheName, providerName string, phase croType.StatusPhase) map[string]string {
	labels := BuildRedisGenericMetricLabels(r, clusterID, cacheName, providerName)
	labels["statusPhase"] = string(phase)
	return labels
}
