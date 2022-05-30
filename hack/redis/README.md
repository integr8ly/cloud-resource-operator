# Redis

Scripts and helpers for working with Redis

## Load Testing Redis AWS (Elasticache)
The deployments is taking from the source `redis-load` generates random entries to redis

#### Prerequisites

Follow this guide to install [the latest version of oc](https://docs.openshift.com/container-platform/4.5/cli_reference/openshift_cli/getting-started-cli.html). There are some oc commands that have only been shown to work on `oc` version `4.5.5` and higher. 

### Preparing the load testing tool

* export the namespace you'll run the load generator pod from

```
export ns=cloud-resource-operator
```

* Create a managed redis instance with `tier` set to `production` that you will run the load test against.

```
cat deploy/crds/integreatly_v1alpha1_redis_cr.yaml | sed "s/type: REPLACE_ME/type: managed/g" | sed "s/tier: development/tier: production/g" | oc apply -f - -n $ns
```

* Start the load generator pod, build the redis-load program locally and copy it to the pod.

```
cd hack/redis
oc run redis-load-test --image redis
export loadpod=$(oc get pod | grep -v "deploy" | grep redis-load-test | awk '{print $1}')
cd redis-load && env GOOS=linux go build  -o redis-load . && chmod +x redis-load && cd ../ && oc cp redis-load/redis-load $loadpod:/data
```

rsh into the load test container

```
oc rsh $loadpod
```

In the load test container you should be able to see that the load testing program has been copied by running it with no flags

```bash
[root@redis-load-test /]# ./redis-load 

INFO[0000] starting redis load pre-reqs                  action="running redis load"
INFO[0000] starting redis load pre-reqs                  action="running prerequisites"
ERRO[0000] host value missing missing                    action="running prerequisites"
  -c, --connections int    The number of simultaneous connections made to the redis server (default 100)
  -h, --host string        The hostname of the redis instance (Required)
      --load-cpu           if true, intensive redis KEYS queries will be run to spike CPU utilization
      --load-data          if true, bulk inserts data into redis. Number of insertions is connections * num-requests
  -n, --num-requests int   The number of requests that will be created by each connection (default 10000)
  -p, --port int           the port of the redis instance (default 6379)
FATA[0000] prerequisites failed host option missing      action="running redis load"
```

### Using the Load Generator Tool

The load testing tool can be used for the following

* Verifying the high memory alerts in Prometheus by bulk inserting lots of random data
* Verifying the high CPU alert by running CPU intensive queries against the server once it is populated with lots of data.

#### Bulk insert data into redis

Setting the `--load-data` flag will cause the program to spin up a number of concurrent connections and connection will insert a number of entries.

```
./redis-load --load-data --host my.host.elasticache.aws.com
```

`-c` sets the number of connections and `-n` sets the number of requests

```
./redis-load --load-data -c 500 -n 10000 --host my.host.elasticache.aws.com
```

Tests have shown that

* 100 connections * 1000 requests creates roughly ~20MB of data
* 100 connections * 10000 requests creates roughly ~214MB of data which is ~50% for a `t2.micro` instance

#### Spike the redis CPU

It's recommended to fill the redis memory to roughly 75% before trying to load the CPU. For a `t2.micro` instance this can be achieved with:

```
./redis-load --load-data -c 500 -n 15000 --host my.host.elasticache.aws.com
```

Setting the `--load-cpu` flag will cause the program to spin up a number of concurrent connections and each connection will perform a number of CPU intensive requests. The request is a redis [KEYS](https://redis.io/commands/keys) command. The complexity of this command is O(n) where n is the number of keys in the cache. You must insert a lot of data first in order to spike the CPU utilization.

```
./redis-load --load-cpu --host my.host.elasticache.aws.com
```

The command runs indefinitely. Interrupt the process with `ctrl + C`.

`-c` sets the number of connections

```
./redis-load --load-cpu -c 200 --host my.host.elasticache.aws.com
```

`200` connections should spike the CPU utilization to ~98% on a `t2.micro` instance.