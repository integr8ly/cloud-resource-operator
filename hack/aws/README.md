# AWS Helpers

### supported_types
Used to check every Availability Zone to see that specific instance types are supported.

We should check for both current instance types used by CRO and their legacy equivalent. 
#### Usage
``` 
./supported_types.sh <<instance type to check>>
```