package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/sirupsen/logrus"
	"reflect"
	"testing"
)

func Test_buildSubnetAddress(t *testing.T) {
	type args struct {
		vpc *ec2.Vpc
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
					VpcId:     aws.String("test"),
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
					VpcId:     aws.String("test"),
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