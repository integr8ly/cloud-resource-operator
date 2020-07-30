# Postgres 

Scripts to load postgres (AWS) with data to easily allow and test alerting

## Usage 
### Load Script
Seed a workshop `postgres` instance
```
make cluster/seed/workshop/postgres
```
Copy `load.sh` file to provisioned postgres workshop pod  
```
 oc cp load.sh <<namespace>>/<<pod name>>:/var/lib/pgsql
```
Run command
``` 
oc exec oc exec <<pod name>> sh /var/lib/pgsql/load.sh <<host>> <<postgres password>> <<size to fill in GiB>> -n <<namespace>> sh /var/lib/pgsql/load.sh <<host>> <<postgres password>> <<size to fill in GiB>>
```

### Clean Script
Seed a workshop `postgres` instance
```
make cluster/seed/workshop/postgres
```
Copy `load.sh` file to provisioned postgres workshop pod  
```
 oc cp clean.sh <<namespace>>/<<pod name>>:/var/lib/pgsql
```
Run command
``` 
oc exec oc exec <<pod name>> sh /var/lib/pgsql/clean.sh <<host>> <<postgres password>>
```