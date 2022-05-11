package aws

import (
	"context"
	configv1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/rds"
)

func Test_ec2TagsToGeneric(t *testing.T) {
	type args struct {
		ec2Tags []*ec2.Tag
	}
	tests := []struct {
		name string
		args args
		want []*tag
	}{
		{
			name: "test convert format",
			args: args{
				ec2Tags: []*ec2.Tag{
					{
						Key:   aws.String("testKey"),
						Value: aws.String("testVal"),
					},
				},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
			},
		},
		{
			name: "test missing keys or values",
			args: args{
				ec2Tags: []*ec2.Tag{
					{
						Value: aws.String("testVal"),
					},
					{
						Key: aws.String("testKey"),
					},
				},
			},
			want: []*tag{
				{
					value: "testVal",
				},
				{
					key: "testKey",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ec2TagsToGeneric(tt.args.ec2Tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ec2TagsToGeneric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_rdsTagsToGeneric(t *testing.T) {
	type args struct {
		rdsTags []*rds.Tag
	}
	tests := []struct {
		name string
		args args
		want []*tag
	}{
		{
			name: "test convert format",
			args: args{
				rdsTags: []*rds.Tag{
					{
						Key:   aws.String("testKey"),
						Value: aws.String("testVal"),
					},
				},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
			},
		},
		{
			name: "test missing keys or values",
			args: args{
				rdsTags: []*rds.Tag{
					{
						Value: aws.String("testVal"),
					},
					{
						Key: aws.String("testKey"),
					},
				},
			},
			want: []*tag{
				{
					value: "testVal",
				},
				{
					key: "testKey",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rdsTagstoGeneric(tt.args.rdsTags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rdsTagstoGeneric() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_genericToEc2Tags(t *testing.T) {
	type args struct {
		tags []*tag
	}
	tests := []struct {
		name string
		args args
		want []*ec2.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: []*ec2.Tag{
				{
					Key:   aws.String("testKey"),
					Value: aws.String("testVal"),
				},
			},
		},
		{
			name: "test missing keys or values",
			args: args{
				tags: []*tag{
					{
						value: "testVal",
					},
					{
						key: "testKey",
					},
				},
			},
			want: []*ec2.Tag{
				{
					Key:   aws.String(""),
					Value: aws.String("testVal"),
				},
				{
					Key:   aws.String("testKey"),
					Value: aws.String(""),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := genericToEc2Tags(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("genericToEc2Tags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_genericToRdsTags(t *testing.T) {
	type args struct {
		tags []*tag
	}
	tests := []struct {
		name string
		args args
		want []*rds.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: []*rds.Tag{
				{
					Key:   aws.String("testKey"),
					Value: aws.String("testVal"),
				},
			},
		},
		{
			name: "test missing keys or values",
			args: args{
				tags: []*tag{
					{
						value: "testVal",
					},
					{
						key: "testKey",
					},
				},
			},
			want: []*rds.Tag{
				{
					Key:   aws.String(""),
					Value: aws.String("testVal"),
				},
				{
					Key:   aws.String("testKey"),
					Value: aws.String(""),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := genericToRdsTags(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("genericToRdsTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_genericToElasticacheTags(t *testing.T) {
	type args struct {
		tags []*tag
	}
	tests := []struct {
		name string
		args args
		want []*elasticache.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: []*elasticache.Tag{
				{
					Key:   aws.String("testKey"),
					Value: aws.String("testVal"),
				},
			},
		},
		{
			name: "test missing keys or values",
			args: args{
				tags: []*tag{
					{
						value: "testVal",
					},
					{
						key: "testKey",
					},
				},
			},
			want: []*elasticache.Tag{
				{
					Key:   aws.String(""),
					Value: aws.String("testVal"),
				},
				{
					Key:   aws.String("testKey"),
					Value: aws.String(""),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := genericToElasticacheTags(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("genericToElasticacheTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_tagsContains(t *testing.T) {
	type args struct {
		tags  []*tag
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
				tags: []*tag{
					{
						key:   "testKey",
						value: "testVal",
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
				tags: []*tag{
					{
						key:   "testKey",
						value: "testVal",
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
			if got := tagsContains(tt.args.tags, tt.args.key, tt.args.value); got != tt.want {
				t.Errorf("tagsContains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mergeTags(t *testing.T) {
	type args struct {
		tags1 []*tag
		tags2 []*tag
	}
	tests := []struct {
		name string
		args args
		want []*tag
	}{
		{
			name: "test success",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey2",
						value: "testVal2",
					},
				},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
				{
					key:   "testKey2",
					value: "testVal2",
				},
			},
		},
		{
			name: "test duplicate tag retrieves first value",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal2",
					},
					{
						key:   "testKey3",
						value: "testVal3",
					},
				},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
				{
					key:   "testKey3",
					value: "testVal3",
				},
			},
		},
		{
			name: "test empty first array",
			args: args{
				tags1: []*tag{},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
			},
		},
		{
			name: "test empty second array",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{},
			},
			want: []*tag{
				{
					key:   "testKey",
					value: "testVal",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeTags(tt.args.tags1, tt.args.tags2)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeTags() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_tagsContainsAll(t *testing.T) {
	type args struct {
		tags1 []*tag
		tags2 []*tag
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test success",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: true,
		},
		{
			name: "test success - different size",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
					{
						key:   "testKey2",
						value: "testVal2",
					},
				},
			},
			want: true,
		},
		{
			name: "test failure",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal2",
					},
				},
			},
			want: false,
		},
		{
			name: "test failure - different size",
			args: args{
				tags1: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
					{
						key:   "testKey2",
						value: "testVal2",
					},
				},
				tags2: []*tag{
					{
						key:   "testKey",
						value: "testVal",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagsContainsAll(tt.args.tags1, tt.args.tags2); got != tt.want {
				t.Errorf("tagsContainsAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultResourceTags(t *testing.T) {
	scheme := runtime.NewScheme()
	err := configv1.AddToScheme(scheme)
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
		want    []*tag
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
						InfrastructureName: defaultInfraName,
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
			want: []*tag{
				{
					key:   "test-key",
					value: "test-value",
				},
				{
					key:   "integreatly.org/clusterID",
					value: "test",
				},
				{
					key:   "integreatly.org/resource-type",
					value: "specType",
				},
				{
					key:   "integreatly.org/resource-name",
					value: "name",
				},
				{
					key:   "red-hat-managed",
					value: "true",
				},
				{
					key:   "integreatly.org/product-name",
					value: "prodName",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := getDefaultResourceTags(tt.args.ctx, tt.args.client, tt.args.specType, tt.args.name, tt.args.prodName)
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
	err := configv1.AddToScheme(scheme)
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
		want    []*tag
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
						InfrastructureName: defaultInfraName,
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
			want: []*tag{
				{
					key:   "test-key",
					value: "test-value",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getUserInfraTags(tt.args.ctx, tt.args.client)
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error in getUserInfraTags(): %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expected %v to equal %v", got, tt.want)
			}
		})
	}
}
