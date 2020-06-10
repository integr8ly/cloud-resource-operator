package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// generic key-value tag
type tag struct {
	key   string
	value string
}

func ec2TagsToGeneric(ec2Tags []*ec2.Tag) []*tag {
	var genericTags []*tag
	for _, ec2Tag := range ec2Tags {
		genericTags = append(genericTags, &tag{key: aws.StringValue(ec2Tag.Key), value: aws.StringValue(ec2Tag.Value)})
	}
	return genericTags
}

func tagsContains(tags []*tag, key, value string) bool {
	for _, tag := range tags {
		if tag.key == key && tag.value == value {
			return true
		}
	}
	return false
}
