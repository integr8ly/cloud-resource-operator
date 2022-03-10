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
# TODO - detach policy with only the required permissions by CRO
aws iam detach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::aws:policy/AdministratorAccess || true
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
# TODO - attach policy with only the required permissions by CRO
aws iam attach-role-policy --role-name $ROLE_NAME --policy-arn arn:aws:iam::aws:policy/AdministratorAccess || true

sed "s|ROLE_ARN|$(get_role_arn)|g" scripts/sts/sts-secret.yaml | oc apply -f - -n $NAMESPACE
