package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"reflect"
	"testing"
)

func Test_buildSubnetAddress(t *testing.T) {
	type args struct {
		vpc *ec2.Vpc
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
				vpc: &ec2.Vpc{
					CidrBlock: aws.String(""),
				},
			},
			wantErr: true,
		},
		{
			name: "test error when cidr mask is greater or equal than 28",
			args: args{
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("127.0.0.1/28"),
				},
			},
			wantErr: true,
		},
		{
			name: "test expected returned networks with /27 source cidr",
			args: args{
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("10.11.128.0/27"),
					VpcId:     aws.String("test"),
				},
			},
			want: []string{
				"10.11.128.16/28",
				"10.11.128.0/28",
			},
			wantErr: false,
		},
		{
			name: "test expected returned networks with /23 source cidr",
			args: args{
				vpc: &ec2.Vpc{
					CidrBlock: aws.String("10.11.128.0/23"),
					VpcId:     aws.String("test"),
				},
			},
			want: []string{
				"10.11.129.240/28",
				"10.11.129.224/28",
				"10.11.129.208/28",
				"10.11.129.192/28",
				"10.11.129.176/28",
				"10.11.129.160/28",
				"10.11.129.144/28",
				"10.11.129.128/28",
				"10.11.129.112/28",
				"10.11.129.96/28",
				"10.11.129.80/28",
				"10.11.129.64/28",
				"10.11.129.48/28",
				"10.11.129.32/28",
				"10.11.129.16/28",
				"10.11.129.0/28",
				"10.11.128.240/28",
				"10.11.128.224/28",
				"10.11.128.208/28",
				"10.11.128.192/28",
				"10.11.128.176/28",
				"10.11.128.160/28",
				"10.11.128.144/28",
				"10.11.128.128/28",
				"10.11.128.112/28",
				"10.11.128.96/28",
				"10.11.128.80/28",
				"10.11.128.64/28",
				"10.11.128.48/28",
				"10.11.128.32/28",
				"10.11.128.16/28",
				"10.11.128.0/28",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSubnetAddress(tt.args.vpc)
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
