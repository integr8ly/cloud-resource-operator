# Cloud Resource Operator - Blob Storage

## Usage

### OpenShift
```
$ make cluster/prepare 
$ make cluster/seed/blobstorage PROVIDER=openshift
```

### AWS
```
$ make cluster/prepare 
$ make cluster/seed/blobstorage PROVIDER=aws
```

### GCP
```
$ make cluster/prepare 
$ make cluster/seed/blobstorage PROVIDER=gcp
```

## Strategy

### AWS
A JSON object containing three keys:
 - `region`, which is the [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region)
 - `createStrategy`, which is a JSON representation of the [`CreateBucketInput` struct](https://docs.aws.amazon.com/sdk-for-go/api/service/s3/#CreateBucketInput)
 - `deleteStrategy`, which accepts a boolean `forceBucketDeletion`. When set to true it will remove the bucket regardless of its contents. When set to false, it will only delete the bucket if it is empty.