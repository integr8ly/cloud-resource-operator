# Redis

Scripts and helpers for working with Redis

## Load Testing Redis AWS (Elasticache)
The deployments is taking from the source `redis-load` generates random entries to redis
### Usage 
#### Prerequisites
```
export ns=<<namespace you want to deploy the load generator>>
export cr=<<name of redis cr>>
```
#### Run Load Generator
``` 
oc process -f redis-load-test.yaml -p HOST=$(oc get secret -n $ns $(oc get redis $cr -n $ns -o jsonpath='{.status.secretRef.name}') -o jsonpath='{.data.uri}' | base64 --decode) -p PORT=6379 -p ENTRIES=<<no of entries>> -p USERS=<<no of concurrent users>> | oc create -f - -n $ns
```
_Note roughly `50000` entries for `1` user will cause a `1%` memory usage jump for standard `cache.t3.micro` instance_

#### Clean up Load Generator 
``` 
oc delete deployment -n $ns
```