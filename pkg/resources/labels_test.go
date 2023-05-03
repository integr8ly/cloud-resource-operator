package resources

import (
	"reflect"
	"testing"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				LabelClusterIDKey:   "testClusterId",
				LabelResourceIDKey:  "testRedisName",
				LabelNamespaceKey:   "testRedisNs",
				LabelInstanceIDKey:  "testCacheName",
				LabelProductNameKey: "testProductName",
				LabelStrategyKey:    "gcp-memorystore",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildGenericMetricLabels(tt.args.r.ObjectMeta, tt.args.clusterID, tt.args.cacheName, tt.args.providerName); !reflect.DeepEqual(got, tt.want) {
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
				LabelClusterIDKey:   "testClusterId",
				LabelResourceIDKey:  "testRedisName",
				LabelNamespaceKey:   "testRedisNs",
				LabelInstanceIDKey:  "testCacheName",
				LabelProductNameKey: "testProductName",
				LabelStrategyKey:    "gcp-memorystore",
				LabelStatusKey:      "nil",
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
				LabelClusterIDKey:   "testClusterId",
				LabelResourceIDKey:  "testRedisName",
				LabelNamespaceKey:   "testRedisNs",
				LabelInstanceIDKey:  "testCacheName",
				LabelProductNameKey: "testProductName",
				LabelStrategyKey:    "gcp-memorystore",
				LabelStatusKey:      "testStatus",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildInfoMetricLabels(tt.args.r.ObjectMeta, tt.args.status, tt.args.clusterID, tt.args.cacheName, tt.args.providerName); !reflect.DeepEqual(got, tt.want) {
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
				LabelClusterIDKey:   "testClusterId",
				LabelResourceIDKey:  "testRedisName",
				LabelNamespaceKey:   "testRedisNs",
				LabelInstanceIDKey:  "testCacheName",
				LabelProductNameKey: "testProductName",
				LabelStrategyKey:    "gcp-memorystore",
				LabelStatusPhaseKey: string(types.PhaseComplete),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildStatusMetricsLabels(tt.args.r.ObjectMeta, tt.args.clusterID, tt.args.cacheName, tt.args.providerName, tt.args.phase); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildRedisStatusMetricsLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasLabel(t *testing.T) {
	type args struct {
		r   *v1alpha1.Redis
		key string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "success label exists",
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
				key: "productName",
			},
			want: true,
		},
		{
			name: "success label does not exist",
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
				key: "missingKey",
			},
			want: false,
		},
		{
			name: "success no labels set",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
					},
				},
				key: "missingKey",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasLabel(tt.args.r, tt.args.key); got != tt.want {
				t.Errorf("HasLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasLabelWithValue(t *testing.T) {
	type args struct {
		r     *v1alpha1.Redis
		key   string
		value string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "success label exists",
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
				key:   "productName",
				value: "testProductName",
			},
			want: true,
		},
		{
			name: "success label exists wrong value",
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
				key:   "productName",
				value: "missingValue",
			},
			want: false,
		},
		{
			name: "success label does not exist",
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
				key:   "missingKey",
				value: "missingValue",
			},
			want: false,
		},
		{
			name: "success no labels set",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
					},
				},
				key:   "missingKey",
				value: "missingValue",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasLabelWithValue(tt.args.r, tt.args.key, tt.args.value); got != tt.want {
				t.Errorf("HasLabelWithValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLabel(t *testing.T) {
	type args struct {
		r   *v1alpha1.Redis
		key string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "success label exists",
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
				key: "productName",
			},
			want: "testProductName",
		},
		{
			name: "success label does not exist",
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
				key: "missingKey",
			},
			want: "",
		},
		{
			name: "success no labels set",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
					},
				},
				key: "missingKey",
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLabel(tt.args.r, tt.args.key); got != tt.want {
				t.Errorf("GetLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddLabel(t *testing.T) {
	type args struct {
		r     *v1alpha1.Redis
		key   string
		value string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "success add new label",
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
				key:   LabelClusterIDKey,
				value: "testClusterId",
			},
		},
		{
			name: "success label already exists",
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
				key:   "productName",
				value: "testProductName",
			},
		},
		{
			name: "success label exists wrong value",
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
				key:   "productName",
				value: "newProductName",
			},
		},
		{
			name: "success no labels set",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
					},
				},
				key:   LabelClusterIDKey,
				value: "testClusterId",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddLabel(tt.args.r, tt.args.key, tt.args.value)
			if !HasLabelWithValue(tt.args.r, tt.args.key, tt.args.value) {
				t.Errorf("AddLabel(); tt.args.r = %v, wanted labels %s:%s", tt.args.r, tt.args.key, tt.args.value)
			}
		})
	}
}

func TestRemoveLabel(t *testing.T) {
	type args struct {
		r   *v1alpha1.Redis
		key string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "success remove label",
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
				key: "productName",
			},
		},
		{
			name: "success label does not exist",
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
				key: "missingKey",
			},
		},
		{
			name: "success no labels set",
			args: args{
				r: &v1alpha1.Redis{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "testRedisName",
						Namespace: "testRedisNs",
					},
				},
				key: "missingKey",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RemoveLabel(tt.args.r, tt.args.key)
			if HasLabel(tt.args.r, tt.args.key) {
				t.Errorf("RemoveLabel(); tt.args.r = %v, label present %s", tt.args.r, tt.args.key)
			}
		})
	}
}
