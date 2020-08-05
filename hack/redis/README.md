# Redis

Scripts and helpers for working with Redis

## Load Testing Redis AWS (Elasticache)
The following is taking from (redis-load)[https://github.com/ciaranRoche/redis-load] which generates random entries to redis
### Usage 
#### Prerequisites
```
export ns=<<namespace you want to deploy the load generator>>
```
#### Run Load Generator
``` 
oc create -f redis-load.yaml -n $ns
oc process redis-load-template -p HOST=<<redis host>> -p PORT=6379 -p ENTRIES=<<no of entries to create>> -p USERS=<<no of concurrent user>> | oc create -f - 
```

#### Clean up Load Generator 
``` 
oc delete deployment -n $ns
oc delete templates redis-load-template -n $ns
```