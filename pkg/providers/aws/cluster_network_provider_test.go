package aws

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

const (
	defaultRHMISubnetTag = "integreatly.org/clusterID"
)

func buildNoneRHMISubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildValidRHMISubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildMultipleValidRHMISubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
			},
		},
		{
			VpcId:            aws.String("testID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultRHMISubnetTag),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func buildNoneVPCAssociatedSubnets() []*ec2.Subnet {
	return []*ec2.Subnet{
		{
			VpcId:            aws.String("notTestID"),
			AvailabilityZone: aws.String("test"),
			Tags: []*ec2.Tag{
				{
					Key:   aws.String(defaultAWSPrivateSubnetTagKey),
					Value: aws.String("1"),
				},
			},
		},
	}
}

func TestNetworkProvider_IsEnabled(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type fields struct {
		logger *logrus.Entry
	}
	type args struct {
		ctx    context.Context
		c      client.Client
		ec2Svc ec2iface.EC2API
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr bool
	}{
		{
			// we expect if no rhmi subnets exist in the cluster vpc isEnabled will return true
			name: "verify isEnabled is true, no rhmi subnets found in cluster vpc",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildNoneRHMISubnets()},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			want:    true,
			wantErr: false,
		},
		{

			//we expect if a single rhmi subnet is found in cluster vpc isEnabled will return true
			name: "verify isEnabled is false, a single rhmi subnet is found in cluster vpc",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildValidRHMISubnets()},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			want:    false,
			wantErr: false,
		},
		{
			// we expect isEnable to return true if more then one rhmi subnet is found in cluster vpc
			name: "verify isEnabled is true, multiple rhmi subnets found in cluster vpc",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildMultipleValidRHMISubnets()},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			want:    false,
			wantErr: false,
		},
		{
			// we always expect subnets to exist in the cluster vpc, this ensures we get an error if none exist
			name: "verify error, if no subnets are found",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{vpcs: buildVpcs()},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
		{
			// we always expect a cluster vpc, this ensures we get an error is none exist
			name: "verify error, if no cluster vpc is found",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
		{
			// we always expect subnets to exist in the cluster vpc,
			// this test ensures an error if subnets exist in the cluster vpc but not associated with the vpc
			name: "verify error, if no subnets found in cluster vpc",
			args: args{
				ctx:    context.TODO(),
				c:      fake.NewFakeClientWithScheme(scheme, buildTestInfra()),
				ec2Svc: &mockEc2Client{vpcs: buildVpcs(), subnets: buildNoneVPCAssociatedSubnets()},
			},
			fields: fields{
				logger: logrus.NewEntry(logrus.StandardLogger()),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NetworkProvider{
				logger: tt.fields.logger,
			}
			got, err := n.IsEnabled(tt.args.ctx, tt.args.c, tt.args.ec2Svc)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsEnabled() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsEnabled() got = %v, want %v", got, tt.want)
			}
		})
	}
}
