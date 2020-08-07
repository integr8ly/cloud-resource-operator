#!/usr/bin/env bash

# we need to insure instances provisioned by CRO are supported in every region
#
# this script gathers all AWS regions and build a list of every Availability Zone
# we then check for supported instance type in each Availability Zone
# unsupported Availability Zones are printed out

INSTANCE_TYPE=$1

declare -a REGION_NAMES
declare -a ZONE_NAMES
declare -a OFFERING_ZONE_NAMES
declare -a OFFERING_ZONE_NAMES

# get all AWS regions
get_regions() {
  REGIONS=$(aws ec2 describe-regions | jq '.Regions')

  for region in $(jq -r '.[]["RegionName"]' <<<"$REGIONS")
  do
    REGION_NAMES=("${REGION_NAMES[@]}" "$region")
  done
}

# for every region get its associated availability zone
# there is no way to list all availability zones without knowing the region
get_availability_zones() {
  AZS=$(aws ec2 describe-availability-zones --region "$region" | jq '.AvailabilityZones')

  for zone in $(jq -r '.[]["ZoneName"]' <<<"$AZS")
  do
    ZONE_NAMES=("${ZONE_NAMES[@]}" "$zone")
  done
}

# get the instance type offerings per region
get_instance_offerings(){
  INSTANCE_OFFERINGS=$(aws ec2 describe-instance-type-offerings --location-type availability-zone  --filters Name=instance-type,Values="$INSTANCE_TYPE" --region "$region" | jq '.InstanceTypeOfferings')

  for offering in $(jq -r '.[]["Location"]' <<<"$INSTANCE_OFFERINGS")
  do
    OFFERING_ZONE_NAMES=("${OFFERING_ZONE_NAMES[@]}" "$offering")
  done
}

# check for regions which do not support offering
check_supported_offerings() {
  instance_supported=true
  for zone in "${ZONE_NAMES[@]}"
  do
    if [[ " ${OFFERING_ZONE_NAMES[*]}" == *" $zone "* ]]; then
      continue
    else
      instance_supported=false
      echo "$zone does not support $INSTANCE_TYPE"
    fi
  done

  if [ "$instance_supported" = true ] ; then
    echo "$INSTANCE_TYPE has full support"
  fi
}

if [ -z "${INSTANCE_TYPE}" ]; then
  echo "required to pass instance type, eg t3.micro"
  exit 1
fi

echo "checking supported offerings for $INSTANCE_TYPE"
get_regions
for region in "${REGION_NAMES[@]}"
do
  get_availability_zones
  get_instance_offerings
done
check_supported_offerings
echo "finished checking supported offerings for $INSTANCE_TYPE in the following regions:"


