# Cloud Resource Operator - Blob Storage

## Usage
To seed a Kubernetes/Openshift cluster with an example Blob Storage resource:
```
$ make cluster/prepare 
$ make cluster/seed/blobstorage
```

### AWS Strategy
A JSON object containing two keys: `region`, which is the [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region), and `strategy`, which is a JSON representation of the [`CreateBucketInput` struct](https://docs.aws.amazon.com/sdk-for-go/api/service/s3/#CreateBucketInput).