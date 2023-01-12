package resources

import (
	"context"
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_tagsContains(t *testing.T) {
	type args struct {
		tags  []*Tag
		key   string
		value string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test success",
			args: args{
				tags: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				key:   "testKey",
				value: "testVal",
			},
			want: true,
		},
		{
			name: "test failure",
			args: args{
				tags: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				key:   "testKey",
				value: "testVal2",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TagsContains(tt.args.tags, tt.args.key, tt.args.value); got != tt.want {
				t.Errorf("tagsContains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mergeTags(t *testing.T) {
	type args struct {
		tags1 []*Tag
		tags2 []*Tag
	}
	tests := []struct {
		name string
		args args
		want []*Tag
	}{
		{
			name: "test success",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey2",
						Value: "testVal2",
					},
				},
			},
			want: []*Tag{
				{
					Key:   "testKey",
					Value: "testVal",
				},
				{
					Key:   "testKey2",
					Value: "testVal2",
				},
			},
		},
		{
			name: "test duplicate tag retrieves first value",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal2",
					},
					{
						Key:   "testKey3",
						Value: "testVal3",
					},
				},
			},
			want: []*Tag{
				{
					Key:   "testKey",
					Value: "testVal",
				},
				{
					Key:   "testKey3",
					Value: "testVal3",
				},
			},
		},
		{
			name: "test empty first array",
			args: args{
				tags1: []*Tag{},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
			},
			want: []*Tag{
				{
					Key:   "testKey",
					Value: "testVal",
				},
			},
		},
		{
			name: "test empty second array",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{},
			},
			want: []*Tag{
				{
					Key:   "testKey",
					Value: "testVal",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeTags(tt.args.tags1, tt.args.tags2)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeTags() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_tagsContainsAll(t *testing.T) {
	type args struct {
		tags1 []*Tag
		tags2 []*Tag
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test success",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
			},
			want: true,
		},
		{
			name: "test success - different size",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
					{
						Key:   "testKey2",
						Value: "testVal2",
					},
				},
			},
			want: true,
		},
		{
			name: "test failure",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal2",
					},
				},
			},
			want: false,
		},
		{
			name: "test failure - different size",
			args: args{
				tags1: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
					{
						Key:   "testKey2",
						Value: "testVal2",
					},
				},
				tags2: []*Tag{
					{
						Key:   "testKey",
						Value: "testVal",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TagsContainsAll(tt.args.tags1, tt.args.tags2); got != tt.want {
				t.Errorf("tagsContainsAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultResourceTags(t *testing.T) {
	scheme := runtime.NewScheme()
	err := configv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx      context.Context
		client   client.Client
		specType string
		name     string
		prodName string
	}
	tests := []struct {
		name    string
		args    args
		want    []*Tag
		wantErr bool
	}{
		{
			name: "failed to get cluster id",
			args: args{
				ctx:      context.TODO(),
				client:   fake.NewFakeClientWithScheme(scheme),
				specType: "",
				name:     "",
				prodName: "",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "successfully retrieved default resource tags",
			args: args{
				ctx: context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme, &configv1.Infrastructure{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						InfrastructureName: "test",
						PlatformStatus: &configv1.PlatformStatus{
							Type: configv1.AWSPlatformType,
							AWS: &configv1.AWSPlatformStatus{
								Region: "eu-west-1",
								ResourceTags: []configv1.AWSResourceTag{
									{
										Key:   "test-key",
										Value: "test-value",
									},
								},
							},
						},
					},
				}),
				specType: "specType",
				name:     "name",
				prodName: "prodName",
			},
			want: []*Tag{
				{
					Key:   "test-key",
					Value: "test-value",
				},
				{
					Key:   "integreatly.org/clusterID",
					Value: "test",
				},
				{
					Key:   "integreatly.org/resource-type",
					Value: "specType",
				},
				{
					Key:   "integreatly.org/resource-name",
					Value: "name",
				},
				{
					Key:   "red-hat-managed",
					Value: "true",
				},
				{
					Key:   "integreatly.org/product-name",
					Value: "prodName",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := GetDefaultResourceTags(tt.args.ctx, tt.args.client, tt.args.specType, tt.args.name, tt.args.prodName)
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error in getDefaultResourceTags(): %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expected %v to equal %v", got, tt.want)
			}
		})
	}
}

func Test_getUserInfraTags(t *testing.T) {
	scheme := runtime.NewScheme()
	err := configv1.Install(scheme)
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx    context.Context
		client client.Client
	}
	tests := []struct {
		name    string
		args    args
		want    []*Tag
		wantErr bool
	}{
		{
			name: "failed to get cluster infrastructure",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "successfully retrieved user infra tags",
			args: args{
				ctx: context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme, &configv1.Infrastructure{
					ObjectMeta: controllerruntime.ObjectMeta{
						Name: "cluster",
					},
					Status: configv1.InfrastructureStatus{
						InfrastructureName: "test",
						PlatformStatus: &configv1.PlatformStatus{
							Type: configv1.AWSPlatformType,
							AWS: &configv1.AWSPlatformStatus{
								Region: "eu-west-1",
								ResourceTags: []configv1.AWSResourceTag{
									{
										Key:   "test-key",
										Value: "test-value",
									},
								},
							},
						},
					},
				}),
			},
			want: []*Tag{
				{
					Key:   "test-key",
					Value: "test-value",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetUserInfraTags(tt.args.ctx, tt.args.client)
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error in getUserInfraTags(): %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expected %v to equal %v", got, tt.want)
			}
		})
	}
}
