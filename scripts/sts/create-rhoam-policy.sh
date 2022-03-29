#!/usr/bin/env bash
# USAGE
# NAMESPACE=cloud-resource-operator ./create-rhoam-policy.sh
#
# Creates the RHOAM role for CRO to assume on STS cluster
#
# PREREQUISITES
# - jq
# - awscli (logged in at the cmd line in order to get the account id)

# Prevents aws cli from opening editor on responses - https://github.com/aws/aws-cli/issues/4992
export AWS_PAGER=""
ROLE_NAME="rhoam_role"
MINIMAL_POLICY_NAME="${ROLE_NAME}_minimal_policy"
CLUSTER_NAME="${CLUSTER_NAME:-defaultsts}"

# Gets the local aws account id
get_account_id() {
  local ACCOUNT_ID=$(aws sts get-caller-identity | jq -r .Account)
  echo "$ACCOUNT_ID"
}

get_role_arn() {
  echo "arn:aws:iam::$(get_account_id):role/$ROLE_NAME"
}

get_cluster_id() {
  local CLUSTER_ID=$(ocm get clusters --parameter search="name like '%$CLUSTER_NAME%'" | jq '.items[].id' -r)
  echo "$CLUSTER_ID"
}

# Delete policy and role
aws iam delete-role-policy --role-name $ROLE_NAME --policy-name $MINIMAL_POLICY_NAME || true
aws iam delete-role --role-name $ROLE_NAME || true

# Create policy and role
# sts:AssumeRole with iam to allow for running CRO locally with this specific iam user
# sts:AssumeRoleWithWebIdentity with federated oidc provider to allow assuming role when running on cluster in pod
# Allows osdCcsAdmin IAM user to assume this role
cat <<EOM >"$ROLE_NAME.json"
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
          "AWS": [
              "arn:aws:iam::$(get_account_id):user/osdCcsAdmin"
          ],
          "Federated": [
              "arn:aws:iam::$(get_account_id):oidc-provider/rh-oidc.s3.us-east-1.amazonaws.com/$(get_cluster_id)"
          ]
      },
      "Action": ["sts:AssumeRole", "sts:AssumeRoleWithWebIdentity"],
      "Condition": {}
    }
  ]
}
EOM
aws iam create-role --role-name $ROLE_NAME --assume-role-policy-document "file://$ROLE_NAME.json" || true

# Create the minimal required policy for CRO
# and attach it to the RHOAM role
cat <<EOM >"$MINIMAL_POLICY_NAME.json"
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "cloudwatch:GetMetricData",
                "ec2:CreateRoute",
                "ec2:DeleteRoute",
                "ec2:DescribeAvailabilityZones",
                "ec2:DescribeInstanceTypeOfferings",
                "ec2:DescribeInstanceTypes",
                "ec2:DescribeRouteTables",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeSubnets",
                "ec2:DescribeVpcPeeringConnections",
                "ec2:DescribeVpcs",
                "elasticache:CreateReplicationGroup",
                "elasticache:DeleteReplicationGroup",
                "elasticache:DescribeCacheClusters",
                "elasticache:DescribeCacheSubnetGroups",
                "elasticache:DescribeReplicationGroups",
                "elasticache:ModifyReplicationGroup",
                "rds:DescribeDBInstances",
                "rds:DescribeDBSnapshots",
                "rds:DescribeDBSubnetGroups",
                "rds:DescribePendingMaintenanceActions",
                "rds:ListTagsForResource",
                "rds:ModifyDBInstance",
                "rds:RemoveTagsFromResource",
                "s3:CreateBucket",
                "s3:DeleteBucket",
                "s3:ListAllMyBuckets",
                "s3:ListBucket",
                "s3:PutBucketPublicAccessBlock",
                "s3:PutBucketTagging",
                "s3:PutEncryptionConfiguration"
            ],
            "Resource": "*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "ec2:CreateSecurityGroup",
                "ec2:CreateSubnet",
                "ec2:CreateTags",
                "ec2:CreateVpc",
                "ec2:CreateVpcPeeringConnection",
                "elasticache:AddTagsToResource",
                "elasticache:CreateCacheSubnetGroup",
                "elasticache:CreateSnapshot",
                "rds:AddTagsToResource",
                "rds:CreateDBInstance",
                "rds:CreateDBSnapshot",
                "rds:CreateDBSubnetGroup"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "aws:RequestTag/red-hat-managed": "true"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "ec2:AcceptVpcPeeringConnection",
                "ec2:AuthorizeSecurityGroupIngress",
                "ec2:CreateSecurityGroup",
                "ec2:CreateSubnet",
                "ec2:CreateVpcPeeringConnection",
                "ec2:DeleteSecurityGroup",
                "ec2:DeleteSubnet",
                "ec2:DeleteVpc",
                "ec2:DeleteVpcPeeringConnection",
                "elasticache:BatchApplyUpdateAction",
                "elasticache:CreateCacheSubnetGroup",
                "elasticache:CreateSnapshot",
                "elasticache:DeleteCacheSubnetGroup",
                "elasticache:DeleteSnapshot",
                "elasticache:DescribeSnapshots",
                "elasticache:DescribeUpdateActions",
                "elasticache:ModifyCacheSubnetGroup",
                "rds:DeleteDBInstance",
                "rds:DeleteDBSnapshot",
                "rds:DeleteDBSubnetGroup"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "aws:ResourceTag/red-hat-managed": "true"
                }
            }
        }
    ]
}
EOM
aws iam put-role-policy --role-name $ROLE_NAME --policy-name rhoam_role_minimal_policy --policy-document "file://$MINIMAL_POLICY_NAME.json" || true

# Create the STS secret in the CRO namespace
sed "s|ROLE_ARN|$(get_role_arn)|g" scripts/sts/sts-secret.yaml | oc apply -f - -n $NAMESPACE
