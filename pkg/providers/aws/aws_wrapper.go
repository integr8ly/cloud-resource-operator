package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

func NewEC2Client(cfg aws.Config) EC2API {
	return &RealEC2Client{
		Client: ec2.NewFromConfig(cfg),
	}
}

type RealEC2Client struct {
	Client *ec2.Client
}

func (r *RealEC2Client) AuthorizeSecurityGroupIngress(ctx context.Context, input *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return r.Client.AuthorizeSecurityGroupIngress(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeVpcPeeringConnections(ctx context.Context, input *ec2.DescribeVpcPeeringConnectionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	return r.Client.DescribeVpcPeeringConnections(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeSubnets(ctx context.Context, input *ec2.DescribeSubnetsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error) {
	return r.Client.DescribeSubnets(ctx, input, optFns...)
}

func (r *RealEC2Client) CreateTags(ctx context.Context, input *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	return r.Client.CreateTags(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeVpcs(ctx context.Context, input *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return r.Client.DescribeVpcs(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeSecurityGroups(ctx context.Context, input *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return r.Client.DescribeSecurityGroups(ctx, input, optFns...)
}

func (r *RealEC2Client) DeleteSecurityGroup(ctx context.Context, input *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	return r.Client.DeleteSecurityGroup(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeVpcPeeringConnection(ctx context.Context, input *ec2.DescribeVpcPeeringConnectionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	return r.Client.DescribeVpcPeeringConnections(ctx, input, optFns...)
}

func (r *RealEC2Client) CreateVpcPeeringConnection(ctx context.Context, input *ec2.CreateVpcPeeringConnectionInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	return r.Client.CreateVpcPeeringConnection(ctx, input, optFns...)
}

func (r *RealEC2Client) AcceptVpcPeeringConnection(ctx context.Context, input *ec2.AcceptVpcPeeringConnectionInput, optFns ...func(*ec2.Options)) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	return r.Client.AcceptVpcPeeringConnection(ctx, input, optFns...)
}

func (r *RealEC2Client) DeleteVpcPeeringConnection(ctx context.Context, input *ec2.DeleteVpcPeeringConnectionInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	return r.Client.DeleteVpcPeeringConnection(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeRouteTables(ctx context.Context, input *ec2.DescribeRouteTablesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error) {
	return r.Client.DescribeRouteTables(ctx, input, optFns...)
}

func (r *RealEC2Client) CreateRoute(ctx context.Context, input *ec2.CreateRouteInput, optFns ...func(*ec2.Options)) (*ec2.CreateRouteOutput, error) {
	return r.Client.CreateRoute(ctx, input, optFns...)
}

func (r *RealEC2Client) DeleteRoute(ctx context.Context, input *ec2.DeleteRouteInput, optFns ...func(*ec2.Options)) (*ec2.DeleteRouteOutput, error) {
	return r.Client.DeleteRoute(ctx, input, optFns...)
}

func (r *RealEC2Client) CreateVpc(ctx context.Context, input *ec2.CreateVpcInput, optFns ...func(*ec2.Options)) (*ec2.CreateVpcOutput, error) {
	return r.Client.CreateVpc(ctx, input, optFns...)
}

func (r *RealEC2Client) DeleteVpc(ctx context.Context, input *ec2.DeleteVpcInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVpcOutput, error) {
	return r.Client.DeleteVpc(ctx, input, optFns...)
}

func (r *RealEC2Client) WaitUntilVpcExists(ctx context.Context, input *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) error {
	return ec2.NewVpcExistsWaiter(r.Client).Wait(ctx, input, 0)
}

func (r *RealEC2Client) CreateSubnet(ctx context.Context, input *ec2.CreateSubnetInput, optFns ...func(*ec2.Options)) (*ec2.CreateSubnetOutput, error) {
	return r.Client.CreateSubnet(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeInstanceTypeOfferings(ctx context.Context, input *ec2.DescribeInstanceTypeOfferingsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstanceTypeOfferingsOutput, error) {
	return r.Client.DescribeInstanceTypeOfferings(ctx, input, optFns...)
}

func (r *RealEC2Client) DescribeAvailabilityZones(ctx context.Context, input *ec2.DescribeAvailabilityZonesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error) {
	return r.Client.DescribeAvailabilityZones(ctx, input, optFns...)
}

func (r *RealEC2Client) CreateSecurityGroup(ctx context.Context, input *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	return r.Client.CreateSecurityGroup(ctx, input, optFns...)
}

func (r *RealEC2Client) DeleteSubnet(ctx context.Context, input *ec2.DeleteSubnetInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSubnetOutput, error) {
	return r.Client.DeleteSubnet(ctx, input, optFns...)
}

// ---------- RDS ----------
func NewRDSClient(cfg aws.Config) RDSAPI {
	return &RealRDSClient{
		Client: rds.NewFromConfig(cfg),
	}
}

type RealRDSClient struct {
	Client *rds.Client
}

func (r *RealRDSClient) CreateDBSubnetGroup(ctx context.Context, input *rds.CreateDBSubnetGroupInput, optFns ...func(*rds.Options)) (*rds.CreateDBSubnetGroupOutput, error) {
	return r.Client.CreateDBSubnetGroup(ctx, input, optFns...)
}

func (r *RealRDSClient) DescribeDBInstances(ctx context.Context, input *rds.DescribeDBInstancesInput, optFns ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return r.Client.DescribeDBInstances(ctx, input, optFns...)
}

func (r *RealRDSClient) ModifyDBSubnetGroup(ctx context.Context, input *rds.ModifyDBSubnetGroupInput, optFns ...func(*rds.Options)) (*rds.ModifyDBSubnetGroupOutput, error) {
	return r.Client.ModifyDBSubnetGroup(ctx, input, optFns...)
}

func (r *RealRDSClient) ListTagsForResource(ctx context.Context, input *rds.ListTagsForResourceInput, optFns ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error) {
	return r.Client.ListTagsForResource(ctx, input, optFns...)
}

func (r *RealRDSClient) RemoveTagsFromResource(ctx context.Context, input *rds.RemoveTagsFromResourceInput, optFns ...func(*rds.Options)) (*rds.RemoveTagsFromResourceOutput, error) {
	return r.Client.RemoveTagsFromResource(ctx, input, optFns...)
}

func (r *RealRDSClient) DeleteDBSubnetGroup(ctx context.Context, input *rds.DeleteDBSubnetGroupInput, optFns ...func(*rds.Options)) (*rds.DeleteDBSubnetGroupOutput, error) {
	return r.Client.DeleteDBSubnetGroup(ctx, input, optFns...)
}

func (r *RealRDSClient) AddTagsToResource(ctx context.Context, input *rds.AddTagsToResourceInput, optFns ...func(*rds.Options)) (*rds.AddTagsToResourceOutput, error) {
	return r.Client.AddTagsToResource(ctx, input, optFns...)
}

func (r *RealRDSClient) DescribeDBSnapshots(ctx context.Context, input *rds.DescribeDBSnapshotsInput, optFns ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error) {
	return r.Client.DescribeDBSnapshots(ctx, input, optFns...)
}

func (r *RealRDSClient) DescribeDBSubnetGroups(ctx context.Context, input *rds.DescribeDBSubnetGroupsInput, optFns ...func(*rds.Options)) (*rds.DescribeDBSubnetGroupsOutput, error) {
	return r.Client.DescribeDBSubnetGroups(ctx, input, optFns...)
}

func (r *RealRDSClient) DescribePendingMaintenanceActions(ctx context.Context, input *rds.DescribePendingMaintenanceActionsInput, optFns ...func(*rds.Options)) (*rds.DescribePendingMaintenanceActionsOutput, error) {
	return r.Client.DescribePendingMaintenanceActions(ctx, input, optFns...)
}

func (r *RealRDSClient) ApplyPendingMaintenanceAction(ctx context.Context, input *rds.ApplyPendingMaintenanceActionInput, optFns ...func(*rds.Options)) (*rds.ApplyPendingMaintenanceActionOutput, error) {
	return r.Client.ApplyPendingMaintenanceAction(ctx, input, optFns...)
}

func (r *RealRDSClient) ModifyDBInstance(ctx context.Context, input *rds.ModifyDBInstanceInput, optFns ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error) {
	return r.Client.ModifyDBInstance(ctx, input, optFns...)
}

// ---------- Elasticache ----------
func NewElasticacheClient(cfg aws.Config) ElastiCacheAPI {
	return &RealElasticacheClient{
		Client: elasticache.NewFromConfig(cfg),
	}
}

type RealElasticacheClient struct {
	Client *elasticache.Client
}

func (r *RealElasticacheClient) CreateCacheSubnetGroup(ctx context.Context, input *elasticache.CreateCacheSubnetGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.CreateCacheSubnetGroupOutput, error) {
	return r.Client.CreateCacheSubnetGroup(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DescribeCacheClusters(ctx context.Context, input *elasticache.DescribeCacheClustersInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheClustersOutput, error) {
	return r.Client.DescribeCacheClusters(ctx, input, optFns...)
}

func (r *RealElasticacheClient) ModifyCacheSubnetGroup(ctx context.Context, input *elasticache.ModifyCacheSubnetGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.ModifyCacheSubnetGroupOutput, error) {
	return r.Client.ModifyCacheSubnetGroup(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DeleteCacheSubnetGroup(ctx context.Context, input *elasticache.DeleteCacheSubnetGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.DeleteCacheSubnetGroupOutput, error) {
	return r.Client.DeleteCacheSubnetGroup(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DescribeCacheSubnetGroups(ctx context.Context, input *elasticache.DescribeCacheSubnetGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeCacheSubnetGroupsOutput, error) {
	return r.Client.DescribeCacheSubnetGroups(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DescribeReplicationGroups(ctx context.Context, input *elasticache.DescribeReplicationGroupsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeReplicationGroupsOutput, error) {
	return r.Client.DescribeReplicationGroups(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DescribeSnapshots(ctx context.Context, input *elasticache.DescribeSnapshotsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeSnapshotsOutput, error) {
	return r.Client.DescribeSnapshots(ctx, input, optFns...)
}

func (r *RealElasticacheClient) CreateSnapshot(ctx context.Context, input *elasticache.CreateSnapshotInput, optFns ...func(*elasticache.Options)) (*elasticache.CreateSnapshotOutput, error) {
	return r.Client.CreateSnapshot(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DeleteSnapshot(ctx context.Context, input *elasticache.DeleteSnapshotInput, optFns ...func(*elasticache.Options)) (*elasticache.DeleteSnapshotOutput, error) {
	return r.Client.DeleteSnapshot(ctx, input, optFns...)
}

func (r *RealElasticacheClient) DescribeUpdateActions(ctx context.Context, input *elasticache.DescribeUpdateActionsInput, optFns ...func(*elasticache.Options)) (*elasticache.DescribeUpdateActionsOutput, error) {
	return r.Client.DescribeUpdateActions(ctx, input, optFns...)
}

func (r *RealElasticacheClient) ModifyReplicationGroup(ctx context.Context, input *elasticache.ModifyReplicationGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.ModifyReplicationGroupOutput, error) {
	return r.Client.ModifyReplicationGroup(ctx, input, optFns...)
}

func (r *RealElasticacheClient) BatchApplyUpdateAction(ctx context.Context, input *elasticache.BatchApplyUpdateActionInput, optFns ...func(*elasticache.Options)) (*elasticache.BatchApplyUpdateActionOutput, error) {
	return r.Client.BatchApplyUpdateAction(ctx, input, optFns...)
}

func (r *RealElasticacheClient) AddTagsToResource(ctx context.Context, input *elasticache.AddTagsToResourceInput, optFns ...func(*elasticache.Options)) (*elasticache.AddTagsToResourceOutput, error) {
	return r.Client.AddTagsToResource(ctx, input, optFns...)
}

func (r *RealElasticacheClient) CreateReplicationGroup(ctx context.Context, input *elasticache.CreateReplicationGroupInput, optFns ...func(*elasticache.Options)) (*elasticache.CreateReplicationGroupOutput, error) {
	return r.Client.CreateReplicationGroup(ctx, input, optFns...)
}
