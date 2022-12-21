package resources

import (
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func TestBuildRedisGenericMetricLabels(t *testing.T) {
	type args struct {
		r            *v1alpha1.Redis
		clusterID    string
		cacheName    string
		providerName string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "success building generic metric labels",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
						Labels: map[string]string{
							"productName": "testProductName",
						},
					},
				},
				clusterID:    "testClusterId",
				cacheName:    "testCacheName",
				providerName: "gcp-memorystore",
			},
			want: map[string]string{
				"clusterID":   "testClusterId",
				"resourceID":  "testRedisName",
				"namespace":   "testRedisNs",
				"instanceID":  "testCacheName",
				"productName": "testProductName",
				"strategy":    "gcp-memorystore",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRedisGenericMetricLabels(tt.args.r, tt.args.clusterID, tt.args.cacheName, tt.args.providerName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildRedisGenericMetricLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRedisInfoMetricLabels(t *testing.T) {
	type args struct {
		r            *v1alpha1.Redis
		status       string
		clusterID    string
		cacheName    string
		providerName string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "success building info metric labels when the status is empty",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
						Labels: map[string]string{
							"productName": "testProductName",
						},
					},
				},
				status:       "",
				clusterID:    "testClusterId",
				cacheName:    "testCacheName",
				providerName: "gcp-memorystore",
			},
			want: map[string]string{
				"clusterID":   "testClusterId",
				"resourceID":  "testRedisName",
				"namespace":   "testRedisNs",
				"instanceID":  "testCacheName",
				"productName": "testProductName",
				"strategy":    "gcp-memorystore",
				"status":      "nil",
			},
		},
		{
			name: "success building info metric labels when the status is not empty",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
						Labels: map[string]string{
							"productName": "testProductName",
						},
					},
				},
				status:       "testStatus",
				clusterID:    "testClusterId",
				cacheName:    "testCacheName",
				providerName: "gcp-memorystore",
			},
			want: map[string]string{
				"clusterID":   "testClusterId",
				"resourceID":  "testRedisName",
				"namespace":   "testRedisNs",
				"instanceID":  "testCacheName",
				"productName": "testProductName",
				"strategy":    "gcp-memorystore",
				"status":      "testStatus",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRedisInfoMetricLabels(tt.args.r, tt.args.status, tt.args.clusterID, tt.args.cacheName, tt.args.providerName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildRedisInfoMetricLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildRedisStatusMetricsLabels(t *testing.T) {
	type args struct {
		r            *v1alpha1.Redis
		clusterID    string
		cacheName    string
		providerName string
		phase        types.StatusPhase
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "success building status metric labels",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
						Labels: map[string]string{
							"productName": "testProductName",
						},
					},
				},
				clusterID:    "testClusterId",
				cacheName:    "testCacheName",
				providerName: "gcp-memorystore",
				phase:        types.PhaseComplete,
			},
			want: map[string]string{
				"clusterID":   "testClusterId",
				"resourceID":  "testRedisName",
				"namespace":   "testRedisNs",
				"instanceID":  "testCacheName",
				"productName": "testProductName",
				"strategy":    "gcp-memorystore",
				"statusPhase": string(types.PhaseComplete),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildRedisStatusMetricsLabels(tt.args.r, tt.args.clusterID, tt.args.cacheName, tt.args.providerName, tt.args.phase); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildRedisStatusMetricsLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}
