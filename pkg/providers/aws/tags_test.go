package aws

import (
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/rds"
)

func Test_ec2TagListToGenericList(t *testing.T) {
	type args struct {
		ec2Tags []*ec2.Tag
	}
	tests := []struct {
		name string
		args args
		want []*resources.Tag
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
			want: []*resources.Tag{
				{
					Key:   "testKey",
					Value: "testVal",
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
			want: []*resources.Tag{
				{
					Value: "testVal",
				},
				{
					Key: "testKey",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ec2TagListToGenericList(tt.args.ec2Tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ec2TagListToGenericList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_rdsTagListToGenericList(t *testing.T) {
	type args struct {
		rdsTags []*rds.Tag
	}
	tests := []struct {
		name string
		args args
		want []*resources.Tag
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
			want: []*resources.Tag{
				{
					Key:   "testKey",
					Value: "testVal",
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
			want: []*resources.Tag{
				{
					Value: "testVal",
				},
				{
					Key: "testKey",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rdsTagListToGenericList(tt.args.rdsTags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rdsTagListToGenericList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_genericListToEc2TagList(t *testing.T) {
	type args struct {
		tags []*resources.Tag
	}
	tests := []struct {
		name string
		args args
		want []*ec2.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*resources.Tag{
					{
						Key:   "testKey",
						Value: "testVal",
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
				tags: []*resources.Tag{
					{
						Value: "testVal",
					},
					{
						Key: "testKey",
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
			if got := genericListToEc2TagList(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("genericListToEc2TagList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_genericToRdsTags(t *testing.T) {
	type args struct {
		tags []*resources.Tag
	}
	tests := []struct {
		name string
		args args
		want []*rds.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*resources.Tag{
					{
						Key:   "testKey",
						Value: "testVal",
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
				tags: []*resources.Tag{
					{
						Value: "testVal",
					},
					{
						Key: "testKey",
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

func Test_genericListToElasticacheTagList(t *testing.T) {
	type args struct {
		tags []*resources.Tag
	}
	tests := []struct {
		name string
		args args
		want []*elasticache.Tag
	}{
		{
			name: "test convert format",
			args: args{
				tags: []*resources.Tag{
					{
						Key:   "testKey",
						Value: "testVal",
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
				tags: []*resources.Tag{
					{
						Value: "testVal",
					},
					{
						Key: "testKey",
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
			if got := genericListToElasticacheTagList(tt.args.tags); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("genericListToElasticacheTagList() = %v, want %v", got, tt.want)
			}
		})
	}
}
