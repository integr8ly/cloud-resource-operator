# Provider - AWS

The AWS provider will reconcile upon the following resources:

- `Postgres` - Reconcile RDS Instances, see [Postgres docs](./postgresql.md) for more details
- `Redis` - Reconcile Elasticache Replication Groups, see [Redis docs](./redis.md) for more details
- `BlobStorage` - Reconcile S3 Buckets, see [BlobStorage docs](./blobstorage.md) for more details
- `PostgresSnapshot` - One-time snapshot of an RDS Instance
- `RedisSnapshot` - One-time snapshot of an Elasticache Replication Group

## Networking/VPC Configuration

RDS and Elasticache resources are provisioned in a VPC. In previous versions of the Cloud Resource Operator, AWS
resources would be bundled in with the OpenShift cluster VPC. Now a standalone VPC is provisioned, dedicated for these
AWS resources.

***Note: To ensure backwards compatibility with older versions of the Cloud Resource Operator, if previous AWS
resources were provisioned in the OpenShift cluster VPC, future resources will also be provisioned in the OpenShift
cluster VPC.***

Networking or VPC configuration for AWS resources is configured via the `_network` key in the AWS strategy ConfigMap.

The value of `_network` has the keys:

- `region`, an [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region)
- `createStrategy`, a [`CreateVpcInput` struct](https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#CreateVpcInput).
Only the `CidrBlock` from this will be used. Any changes to this config after VPC creation will be ignored.
- `deleteStrategy`, this is currently unused