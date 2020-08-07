# AWS Helpers

### supported_types
Used to check every Availability Zone to see that specific instance types are supported. To match Availability Zones from the scripts result, to its region please follow [AWS documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/using-regions-availability-zones.html#concepts-available-regions).

We should check for both current instance types used by CRO and their legacy equivalent. 

#### Prerequisites

- [aws-cli](https://aws.amazon.com/cli/) 

Ensure you have installed and configured the `aws-cli` to the account you wish to check.

_NOTE: This script only has the ability to check regions which the account has opted into_
#### Usage
``` 
./supported_types.sh <<instance type to check>>
```