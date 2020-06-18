package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"reflect"
	"testing"
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
