package aws

import (
	"reflect"
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
