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

## STS Mode
The AWS provider supports STS authentication for AWS APIs.

To configure CRO to use STS, create a secret named `sts-credentials` in the CRO namespace containing the token file path and role ARN to assume. 

```
role_name = arn:...:role/some-role-name
web_identity_token_file = /var/run/secrets/openshift/serviceaccount
```

CRO will detect this secret is present and will try to assume the role locally and when running in a pod on cluster to
interact with AWS APIs.

This role must have a policy attached with the minimum permissions for CRO to manage the AWS resources.
If you are logged in locally to the cluster via `oc` and also to the aws account in `awscli`, you can run `make setup/sts` 
to create this minimal policy and role to the aws account and the secret onto the CRO namespace.

### Blobstorage
When running in STS mode, the Blobstorage provisioned by CRO will not provide access credentials to these S3 buckets. This
is by design as CRO should not provide credentials typically (it is through the Cloud Credential Operator in a non-sts cluster).
If this is provided by CRO, there will be a permission increase around IAM management. In addition, providing long-lasting credentials
defeats the purpose of STS.

As such, Pods requiring access to these provisioned S3 buckets should itself use STS authentication with a Role with minimum 
S3 permissions and use the information provided by the Blobstorage secret to interact with these S3 buckets.