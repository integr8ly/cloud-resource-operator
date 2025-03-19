package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/mock"
	"reflect"
	"testing"
	"unsafe"

	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_buildSubnetAddress(t *testing.T) {
	type args struct {
		vpc    *ec2types.Vpc
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
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String(""),
				},
			},
			wantErr: true,
		},
		{
			name: "test error when cidr mask is greater or equal than 27",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String("127.0.0.1/27"),
				},
			},
			wantErr: true,
		},
		{
			name: "test expected returned networks with /26 source cidr",
			args: args{
				logger: logrus.NewEntry(logrus.StandardLogger()),
				vpc: &ec2types.Vpc{
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
				vpc: &ec2types.Vpc{
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
		want    []*ec2types.Tag
		wantErr bool
	}{
		{
			name: "failed to get cluster infrastructure",
			args: args{
				ctx:    context.TODO(),
				client: moqClient.NewSigsClientMoqWithScheme(scheme),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "successfully retrieved user infra tags",
			args: args{
				ctx: context.TODO(),
				client: moqClient.NewSigsClientMoqWithScheme(scheme, &configv1.Infrastructure{
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
			want: []*ec2types.Tag{
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

func Test_createPrivateSubnet(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	type args struct {
		ctx       context.Context
		c         client.Client
		ec2Client *ec2.Client
		vpc       *ec2types.Vpc
		logger    *logrus.Entry
		zone      string
		sub       *ec2types.Subnet
	}
	tests := []struct {
		name    string
		args    args
		want    *ec2types.Subnet
		wantErr bool
	}{
		{
			name: "failed to build subnet address",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					// TODO verify , used to return nil before aws-go-sdk-v2 change now it returns a mock ec2 client.
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String(""),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "us-east-1",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error creating new subnet",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					//Todo confirm the genericAWSError passes
					mockEc2.On("CreateSubnet", mock.Anything, mock.Anything, mock.Anything).Return(nil, genericAWSError)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))

				}(),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "us-east-1",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error tagging private subnet",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "us-east-1",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "error creating new subnet - subnet is nil",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					// TODO verify error change for v2
					err := &smithy.GenericAPIError{
						Code:    "InvalidSubnet.Conflict",
						Message: "Subnet conflict error",
					}
					mockEc2.On("CreateSubnet", mock.Anything, mock.Anything, mock.Anything).Return(nil, err)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "us-east-1",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "successfully create subnet",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				ec2Client: func() *ec2.Client {
					mockEc2 := new(mockEc2Client)
					mockEc2.On("DescribeVpcs", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeVpcsOutput{
						Vpcs: []ec2types.Vpc{
							*buildValidStandaloneVPC(validCIDRTwentySix),
						},
					}, nil)
					mockEc2.On("CreateVpc", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.CreateVpcOutput{
						Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
					}, nil)
					mockEc2.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					mockEc2.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					mockEc2.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					mockEc2.On("DescribeRouteTables", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeRouteTablesOutput{
						RouteTables: []ec2types.RouteTable{
							*buildMockEc2RouteTable(nil),
						},
					}, nil)
					mockEc2.On("DescribeSubnets", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeSubnetsOutput{
						Subnets: buildValidBundleSubnets(),
					}, nil)
					mockEc2.On("DescribeAvailabilityZones", mock.Anything, mock.Anything, mock.Anything).Return(&ec2.DescribeAvailabilityZonesOutput{
						AvailabilityZones: buildSortedStandaloneAZs(),
					}, nil)
					return (*ec2.Client)(unsafe.Pointer(mockEc2))
				}(),
				vpc: &ec2types.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "eu-west-1",
			},
			want: &ec2types.Subnet{
				AvailabilityZone: aws.String("test-zone-1"),
				CidrBlock:        aws.String("10.0.0.0/27"),
				SubnetId:         aws.String("test-id-1"),
				Tags: []ec2types.Tag{
					{
						Key:   aws.String("kubernetes.io/role/internal-elb"),
						Value: aws.String("1"),
					},
					{
						Key:   aws.String("integreatly.org/clusterID"),
						Value: aws.String("test"),
					},
					{
						Key:   aws.String("Name"),
						Value: aws.String("Cloud Resource Subnet"),
					},
					{
						Key:   aws.String("red-hat-managed"),
						Value: aws.String("true"),
					},
				},
				VpcId: aws.String("standaloneID"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := createPrivateSubnet(tt.args.ctx, tt.args.c, *tt.args.ec2Client, tt.args.vpc, tt.args.logger, tt.args.zone)
			if (err != nil) != tt.wantErr {
				t.Errorf("createPrivateSubnet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createPrivateSubnet() = %v, want %v", got, tt.want)
			}
		})
	}
}
