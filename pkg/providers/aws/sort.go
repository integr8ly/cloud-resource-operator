package aws

import "github.com/aws/aws-sdk-go/service/ec2"

type azByZoneName []*ec2.AvailabilityZone

func (z azByZoneName) Len() int           { return len(z) }
func (z azByZoneName) Less(i, j int) bool { return *z[i].ZoneName < *z[j].ZoneName }
func (z azByZoneName) Swap(i, j int)      { z[i], z[j] = z[j], z[i] }
