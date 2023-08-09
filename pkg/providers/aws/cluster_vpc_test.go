package aws

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
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
		want    []*ec2.Tag
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

func Test_createPrivateSubnet(t *testing.T) {
	scheme, err := buildTestSchemePostgresql()
	if err != nil {
		logrus.Fatal(err)
		t.Fatal("failed to build scheme", err)
	}

	type args struct {
		ctx    context.Context
		c      client.Client
		ec2Svc ec2iface.EC2API
		vpc    *ec2.Vpc
		logger *logrus.Entry
		zone   string
		sub    *ec2.Subnet
	}
	tests := []struct {
		name    string
		args    args
		want    *ec2.Subnet
		wantErr bool
	}{
		{
			name: "failed to build subnet address",
			args: args{
				ctx:    context.TODO(),
				c:      moqClient.NewSigsClientMoqWithScheme(scheme),
				ec2Svc: nil,
				vpc: &ec2.Vpc{
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
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRTwentySix),
							},
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{
							Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
						}, nil
					}
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildSortedStandaloneAZs(),
						}, nil
					}
					ec2Client.createSubnetFn = func(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
						return nil, genericAWSError
					}
				}),
				vpc: &ec2.Vpc{
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
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRTwentySix),
							},
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{
							Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
						}, nil
					}
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildSortedStandaloneAZs(),
						}, nil
					}
				}),
				vpc: &ec2.Vpc{
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
			name: "subnet is nil",
			args: args{
				ctx: context.TODO(),
				c:   moqClient.NewSigsClientMoqWithScheme(scheme, buildTestInfra()),
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRTwentySix),
							},
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{
							Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
						}, nil
					}
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildSortedStandaloneAZs(),
						}, nil
					}
					ec2Client.createSubnetFn = func(input *ec2.CreateSubnetInput) (*ec2.CreateSubnetOutput, error) {
						return nil, awserr.New("InvalidSubnet.Conflict", "Subnet conflict error", nil)
					}

				}),
				vpc: &ec2.Vpc{
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
				ec2Svc: buildMockEc2Client(func(ec2Client *mockEc2Client) {
					ec2Client.describeVpcsFn = func(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
						return &ec2.DescribeVpcsOutput{
							Vpcs: []*ec2.Vpc{
								buildValidStandaloneVPC(validCIDRTwentySix),
							},
						}, nil
					}
					ec2Client.createVpcFn = func(input *ec2.CreateVpcInput) (*ec2.CreateVpcOutput, error) {
						return &ec2.CreateVpcOutput{
							Vpc: buildValidStandaloneVPC(validCIDRTwentySix),
						}, nil
					}
					ec2Client.subnets = buildStandaloneVPCAssociatedSubnets(defaultValidSubnetMaskOneA, defaultValidSubnetMaskOneB)
					ec2Client.firstSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdOne, defaultAzIdOne, defaultValidSubnetMaskOneA)
					ec2Client.secondSubnet = buildSubnet(defaultStandaloneVpcId, defaultSubnetIdTwo, defaultAzIdTwo, defaultValidSubnetMaskOneB)
					ec2Client.describeRouteTablesFn = func(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
						return &ec2.DescribeRouteTablesOutput{
							RouteTables: []*ec2.RouteTable{
								buildMockEc2RouteTable(nil),
							},
						}, nil
					}
					ec2Client.describeSubnetsFn = func(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
						return &ec2.DescribeSubnetsOutput{
							Subnets: buildValidBundleSubnets(),
						}, nil
					}
					ec2Client.describeAvailabilityZonesFn = func(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
						return &ec2.DescribeAvailabilityZonesOutput{
							AvailabilityZones: buildSortedStandaloneAZs(),
						}, nil
					}
				}),
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String(mockNetworkVpcId),
				},
				logger: logrus.NewEntry(logrus.StandardLogger()),
				zone:   "eu-west-1",
			},
			want: &ec2.Subnet{
				AvailabilityZone: aws.String("test-zone-1"),
				CidrBlock:        aws.String("10.0.0.0/27"),
				SubnetId:         aws.String("test-id-1"),
				Tags: []*ec2.Tag{
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
			got, err := createPrivateSubnet(tt.args.ctx, tt.args.c, tt.args.ec2Svc, tt.args.vpc, tt.args.logger, tt.args.zone)
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
