package gcp

import (
	"context"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

func Test_buildDefaultRedisTags(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		client client.Client
		r      *v1alpha1.Redis
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "failure building default gcp redis tags because the redis cr is nil",
			args:    args{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "success building default redis tags",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme, buildTestGcpInfrastructure(nil)),
				r: &v1alpha1.Redis{
					ObjectMeta: v1.ObjectMeta{
						Name: testName,
					},
					Spec: types.ResourceTypeSpec{
						Type: "testType",
					},
				},
			},
			want: map[string]string{
				"integreatly-org_clusterid":     gcpTestClusterName,
				"integreatly-org_resource-name": "testname",
				"integreatly-org_resource-type": "testtype",
				resources.TagManagedKey:         "true",
			},
			wantErr: false,
		},
		{
			name: "failure building default redis tags",
			args: args{
				client: moqClient.NewSigsClientMoqWithScheme(scheme),
				r: &v1alpha1.Redis{
					ObjectMeta: v1.ObjectMeta{
						Name: testName,
					},
					Spec: types.ResourceTypeSpec{
						Type: "testType",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildDefaultRedisTags(context.TODO(), tt.args.client, tt.args.r)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildDefaultRedisTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildDefaultRedisTags() got = %v, want %v", got, tt.want)
			}
		})
	}
}
