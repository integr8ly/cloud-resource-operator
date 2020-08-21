# Postgres 

Scripts and helpers for working with postgres 

## Load Testing Postgres AWS (RDS)
### Usage 
#### Prerequisites
```
export ns=cloud-resources-load-test
# Spin up workshop `postgres` instance
make cluster/prepare NAMESPACE=$ns
make cluster/seed/workshop/postgres NAMESPACE=$ns
make run NAMESPACE=$ns


# In another terminal window run
export ns=cloud-resources-load-test
export aws_rds_db_host=<your-db-host.rds.amazonaws.com>
export aws_rds_db_password=<password>


# Get postgres workshop pod name
export pod_name=$(oc get pods -n $ns -o jsonpath='{.items[0].metadata.name}')
```
#### Load Script


```
# Size to fill in GiB
export load_data_size=<number>

# Copy `load.sh` file to provisioned postgres workshop pod  
oc cp load.sh $ns/$pod_name:/var/lib/pgsql
```
Run command
``` 
oc exec $pod_name sh /var/lib/pgsql/load.sh $aws_rds_db_host $aws_rds_db_password $load_data_size -n $ns
```
#### CPU Util Script
Copy Script 
```
oc cp cpuUtil.sh $ns/$pod_name:/var/lib/pgsql
```
Run command
```
oc exec $pod_name sh /var/lib/pgsql/cpuUtil.sh $aws_rds_db_host $aws_rds_db_password -n $ns
```
####Memory Usage Scripts
Copy Scripts
```
oc cp memUsageInsert.sh $ns/$pod_name:/var/lib/pgsql
oc cp memUsageExec.sh $ns/$pod_name:/var/lib/pgsql
```

Insert Data
```
oc exec $pod_name sh /var/lib/pgsql/memUsageInsert.sh $aws_rds_db_host $aws_rds_db_password -n $ns
```

Increase Memory Usage
```
# this script will use up a lot of freeable memory, but takes only about
# 5 minutes total to run which will not reliably trigger alerts 
oc exec $pod_name sh /var/lib/pgsql/memUsageExec.sh $aws_rds_db_host $aws_rds_db_password -n $ns
```

#### Clean Script
Copy `clean.sh` file to provisioned postgres workshop pod  
```
 oc cp clean.sh $ns/$pod_name:/var/lib/pgsql
```
Run command
``` 
oc exec $pod_name sh /var/lib/pgsql/clean.sh $aws_rds_db_host $aws_rds_db_password -n $ns
```