# Cloud Resource Operator - SMTP 

## Usage
To seed a Kubernetes/Openshift cluster with an example SMTP resource:
```
$ make cluster/prepare 
$ make cluster/seed/<<workshop or managed>>/smtp
```
### AWS Strategy
A JSON object containing a `region` key, which is a supported [AWS region code](https://docs.aws.amazon.com/general/latest/gr/rande.html#ses_region).   