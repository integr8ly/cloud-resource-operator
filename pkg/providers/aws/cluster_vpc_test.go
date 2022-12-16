package aws

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	configv1 "github.com/integr8ly/cloud-resource-operator/apis/config/v1"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

func Test_buildSubnetAddress(t *testing.T) {
	type args struct {
		vpc    *ec2.Vpc
		logger *logrus.Entry
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "test failure when cidr is not provided",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2.Vpc{
					CidrBlock: aws.String(""),
				},
			},
			wantErr: true,
		},
		{
			name: "test error when cidr mask is greater or equal than 27",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("127.0.0.1/27"),
				},
			},
			wantErr: true,
		},
		{
			name: "test expected returned networks with /26 source cidr",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("10.11.128.0/26"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
			},
			want: []string{
				"10.11.128.32/27",
				"10.11.128.0/27",
			},
			wantErr: false,
		},
		{
			name: "test expected returned networks with /23 source cidr",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
			},
			want: []string{
				"10.11.129.224/27",
				"10.11.129.192/27",
				"10.11.129.160/27",
				"10.11.129.128/27",
				"10.11.129.96/27",
				"10.11.129.64/27",
				"10.11.129.32/27",
				"10.11.129.0/27",
				"10.11.128.224/27",
				"10.11.128.192/27",
				"10.11.128.160/27",
				"10.11.128.128/27",
				"10.11.128.96/27",
				"10.11.128.64/27",
				"10.11.128.32/27",
				"10.11.128.0/27",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSubnetAddress(tt.args.vpc, tt.args.logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildSubnetAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var gotStr []string
			for _, n := range got {
				gotStr = append(gotStr, n.String())
			}
			if !reflect.DeepEqual(gotStr, tt.want) {
				t.Errorf("buildSubnetAddress() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDefaultSubnetTags(t *testing.T) {
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
		want    []*ec2.Tag
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
			want: []*ec2.Tag{
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
				{
					Key:   aws.String("integreatly.org/clusterID"),
					Value: aws.String("test"),
				},
				{
					Key:   aws.String(resources.TagDisplayName),
					Value: aws.String(defaultSubnetNameTagValue),
				},
				{
					Key:   aws.String(resources.TagManagedKey),
					Value: aws.String(resources.TagManagedVal),
				},
				{
					Key:   aws.String("test-key"),
					Value: aws.String("test-value"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getDefaultSubnetTags(tt.args.ctx, tt.args.client)
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error in getDefaultSubnetTags(): %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expected %v to equal %v", got, tt.want)
			}
		})
	}
}
