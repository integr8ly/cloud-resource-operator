#!/bin/bash
# Usage :
# ./create-rosa-sts.sh <cluster name>

set -eux


CLUSTER_NAME=$1
if [[ -z $CLUSTER_NAME ]]; then
  echo "usage: $0 <cluster name>"
  exit 1
fi

name=$CLUSTER_NAME
aws_account_id=$(aws sts get-caller-identity | jq -r .Account)
version=4.7.11
region=us-east-1

rm -rf credrequests
mkdir -p credrequests
echo "extracting credentials requests..."
oc adm release extract quay.io/openshift-release-dev/ocp-release:${version:0:3}.0-x86_64 \
    --credentials-requests \
    --cloud=aws \
    --to credrequests
cat credrequests/0000*.yaml > credrequests/${version:0:3}.yaml
rm -f credrequests/0000*.yaml

curl -s https://gist.githubusercontent.com/sam-nguyen7/6813a665b7b7b65b10e2cfe2311dbcce/raw/e3c805cc9e864b06e0d15e0ab4be8299746b2247/0000_50_managed-velero-operator_06_credentials-request.yaml > credrequests/0000_50_managed-velero-operator_06_credentials-request.yaml

rosa login --env=staging

cat << EOM > OSDCCSAdmin_IAM_role.json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
          "AWS": [
              "arn:aws:iam::644306948063:role/RH-Managed-OpenShift-Installer"
          ]
      },
      "Action": "sts:AssumeRole",
      "Condition": {}
    }
  ]
}
EOM

# TODO: check if policy and/or role already exist and delete(?)
# TODO: handle errors in commands below
aws iam create-role --role-name delegate-admin-OSDCCSAdmin --assume-role-policy-document file://OSDCCSAdmin_IAM_role.json || true
aws iam attach-role-policy --role-name delegate-admin-OSDCCSAdmin --policy-arn arn:aws:iam::aws:policy/AdministratorAccess || true


cat << EOM > OSDCCSAdmin-Master_IAM_role.json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOM
cat << EOM > OSDCCSAdmin-Master_IAM_role_policy.json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "sts:AssumeRole",
                "ec2:AttachVolume",
                "ec2:AuthorizeSecurityGroupIngress",
                "ec2:CreateSecurityGroup",
                "ec2:CreateTags",
                "ec2:CreateVolume",
                "ec2:DeleteSecurityGroup",
                "ec2:DeleteVolume",
                "ec2:Describe*",
                "ec2:DetachVolume",
                "ec2:ModifyInstanceAttribute",
                "ec2:ModifyVolume",
                "ec2:RevokeSecurityGroupIngress",
                "elasticloadbalancing:AddTags",
                "elasticloadbalancing:AttachLoadBalancerToSubnets",
                "elasticloadbalancing:ApplySecurityGroupsToLoadBalancer",
                "elasticloadbalancing:CreateListener",
                "elasticloadbalancing:CreateLoadBalancer",
                "elasticloadbalancing:CreateLoadBalancerPolicy",
                "elasticloadbalancing:CreateLoadBalancerListeners",
                "elasticloadbalancing:CreateTargetGroup",
                "elasticloadbalancing:ConfigureHealthCheck",
                "elasticloadbalancing:DeleteListener",
                "elasticloadbalancing:DeleteLoadBalancer",
                "elasticloadbalancing:DeleteLoadBalancerListeners",
                "elasticloadbalancing:DeleteTargetGroup",
                "elasticloadbalancing:DeregisterInstancesFromLoadBalancer",
                "elasticloadbalancing:DeregisterTargets",
                "elasticloadbalancing:Describe*",
                "elasticloadbalancing:DetachLoadBalancerFromSubnets",
                "elasticloadbalancing:ModifyListener",
                "elasticloadbalancing:ModifyLoadBalancerAttributes",
                "elasticloadbalancing:ModifyTargetGroup",
                "elasticloadbalancing:ModifyTargetGroupAttributes",
                "elasticloadbalancing:RegisterInstancesWithLoadBalancer",
                "elasticloadbalancing:RegisterTargets",
                "elasticloadbalancing:SetLoadBalancerPoliciesForBackendServer",
                "elasticloadbalancing:SetLoadBalancerPoliciesOfListener",
                "kms:DescribeKey"
            ],
            "Resource": "*"
        }
    ]
}
EOM


# TODO: check if policy and/or role already exist and delete(?)
# TODO: handle errors in commands below
aws iam create-role --role-name delegate-admin-OSDCCSAdmin-Master --assume-role-policy-document file://OSDCCSAdmin-Master_IAM_role.json || true #--permissions-boundary ${permissions_boundary_arn}
aws iam put-role-policy --role-name delegate-admin-OSDCCSAdmin-Master --policy-name delegate-admin-osdCCSAdmin-Master --policy-document file://OSDCCSAdmin-Master_IAM_role_policy.json || true


cat <<EOM > OSDCCSAdmin-Worker_IAM_role.json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"       
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
EOM
cat << EOM > OSDCCSAdmin-Worker_IAM_role_policy.json 
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "sts:AssumeRole",
                "ec2:DescribeInstances",
                "ec2:DescribeRegions"
            ],
            "Resource": "*"
        }
    ]
}
EOM

# TODO: check if policy and/or role already exist and delete(?)
# TODO: handle errors in commands below
aws iam create-role --role-name delegate-admin-OSDCCSAdmin-Worker --assume-role-policy-document file://OSDCCSAdmin-Worker_IAM_role.json  || true #--permissions-boundary ${permissions_boundary_arn}
aws iam put-role-policy --role-name delegate-admin-OSDCCSAdmin-Worker --policy-name delegate-admin-osdCCSAdmin-Worker --policy-document file://OSDCCSAdmin-Worker_IAM_role_policy.json  || true

echo "waiting for delegate-admin-OSDCCSAdmin role to become ready"
sleep 15

# Add this if candidate build is used:
# --channel-group candidate \ 
rosa create cluster \
  -c ${name} \
  --region ${region} \
  --disable-scp-checks \
  --version ${version} \
  --role-arn arn:aws:iam::${aws_account_id}:role/delegate-admin-OSDCCSAdmin \
  --operator-iam-roles=aws-cloud-credentials,openshift-machine-api,arn:aws:iam::${aws_account_id}:role/delegate-admin-openshift-machine-api-aws-cloud-credentials \
  --operator-iam-roles=cloud-credential-operator-iam-ro-creds,openshift-cloud-credential-operator,arn:aws:iam::${aws_account_id}:role/delegate-admin-openshift-cloud-credential-operator-cloud-credent \
  --operator-iam-roles=installer-cloud-credentials,openshift-image-registry,arn:aws:iam::${aws_account_id}:role/delegate-admin-openshift-image-registry-installer-cloud-credenti \
  --operator-iam-roles=cloud-credentials,openshift-ingress-operator,arn:aws:iam::${aws_account_id}:role/delegate-admin-openshift-ingress-operator-cloud-credentials \
  --operator-iam-roles=ebs-cloud-credentials,openshift-cluster-csi-drivers,arn:aws:iam::${aws_account_id}:role/delegate-admin-openshift-cluster-csi-drivers-ebs-cloud-credentia \
  --tags cluster-name:${name},cluster-version:${version}

echo "waiting for rosa create"
sleep 15

cluster_id=$(rosa describe cluster -c ${name} | grep ^ID | awk '{print $2}')
set +o pipefail
thumbprint=$(openssl s_client \
  -servername ${cluster_id}-oidc.s3.${region}.amazonaws.com \
  -showcerts \
  -connect ${cluster_id}-oidc.s3.${region}.amazonaws.com:443 </dev/null 2>&1 |
  openssl x509 \
  -fingerprint \
  -noout |
  tail -n1 |
  sed 's/SHA1 Fingerprint=//' |
  sed 's/://g'
)
if [[ ${PIPESTATUS[0]} -gt 0 ]]; then
  echo "Non-zero pipestatus: ${PIPESTATUS[0]}"
  echo "Continuing..."
fi

set -o pipefail
if [[ -z $thumbprint ]]; then
  echo "Nil thumbprint is a problem. Exiting"
  exit 1
fi

aws iam create-open-id-connect-provider \
  --url https://rh-oidc-staging.s3.us-east-1.amazonaws.com/${cluster_id} \
  --client-id-list openshift sts.amazonaws.com \
  --thumbprint-list ${thumbprint}


# TODO: improve policy and role removal below, perhaps move to the same loop where it's being created
roles=(
  "delegate-admin-openshift-cloud-credential-operator-cloud-credent" \
  "delegate-admin-openshift-cluster-csi-drivers-ebs-cloud-credentia" \
  "delegate-admin-openshift-image-registry-installer-cloud-credenti" \
  "delegate-admin-openshift-ingress-operator-cloud-credentials" \
  "delegate-admin-openshift-machine-api-aws-cloud-credentials" \
  "delegate-admin-openshift-velero-managed-velero-operator-iam-cred"
)
echo "deleting conflicting IAM roles..."
for role in "${roles[@]}"; do
  echo "deleting role $role"
  aws iam detach-role-policy --role-name $role --policy-arn arn:aws:iam::${aws_account_id}:policy/${role} || true
  # aws iam delete-role-policy --role-name $role --policy-name $role || true
  aws iam delete-policy --policy-arn arn:aws:iam::${aws_account_id}:policy/${role} || true
  aws iam delete-role --role-name $role || true
done
sleep 10

rm -rf iam_assets
mkdir -p iam_assets
cd iam_assets

ccoctl aws create-iam-roles \
  --credentials-requests-dir ../credrequests/ \
  --identity-provider-arn arn:aws:iam::${aws_account_id}:oidc-provider/rh-oidc-staging.s3.us-east-1.amazonaws.com/${cluster_id} \
  --name delegate-admin \
  --region ${region} \
  --dry-run

for role in `find . -name "*-role.json"`
do 
  policy=$(sed -e 's/05-/06-/' -e 's/role/policy/' <<< ${role})
  role_name=$(grep RoleName ${policy} | awk '{print $2}' | awk -F '"' '{print $2}')
  aws iam create-role --cli-input-json file://${role} #--permissions-boundary ${permissions_boundary_arn}
  sed -i.bak '/RoleName/d' ${policy}
  policy_arn=$(aws iam create-policy --cli-input-json file://$policy | grep Arn | awk '{print $2}' | awk -F '"' '{print $2}')
  aws iam attach-role-policy --role-name $role_name --policy-arn $policy_arn
  rm ${policy}
  mv ${policy}.bak ${policy}
  sleep 5 # Prevents AWS Rate limiting
done

