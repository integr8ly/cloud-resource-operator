package e2e

import (
	goctx "context"
	"fmt"
	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	bv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	errorUtil "github.com/pkg/errors"
)

const (
	redisConnectionJobName = "redis-connection-job"
)

// verify a connection can be made to redis instance
func VerifyRedisConnectionTest(t TestingTB, ctx *TestingContext, namespace string) error {
	// get posgres secret
	sec, err := getRedisSecret(t, ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// create redis cli connection string
	rcliCommand := []string{
		"container-entrypoint",
		"bash",
		"-c",
		fmt.Sprintf("redis-cli -h %s -p %s", sec.Data["uri"], sec.Data["port"])}

	// create redis connection job
	rj := ConnectionJob(buildRedisContainer(rcliCommand), redisConnectionJobName, namespace)
	if err := ctx.Client.Create(goctx.TODO(), rj); err != nil {
		return errorUtil.Wrapf(err, "could not create redis connection job")
	}

	// poll redis connection job for success
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisConnectionJobName}, rj); err != nil {
			return true, errorUtil.Wrapf(err, "could not get connection job")
		}
		for _, s := range rj.Status.Conditions {
			if s.Type == bv1.JobComplete {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	return nil
}

// return redis secret
func getRedisSecret(t TestingTB, ctx *TestingContext, namespace string) (v1.Secret, error) {
	sec := v1.Secret{}
	rcr := &v1alpha1.Redis{}
	// poll cr for complete status phase
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rcr); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get resource")
	}
	t.Logf("postgres status phase %s", rcr.Status.Phase)

	// get created secret
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: rcr.Status.SecretRef.Namespace, Name: rcr.Status.SecretRef.Name}, &sec); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get secret")
	}

	return sec, nil
}

// return redis v1 container
func buildRedisContainer(command []string) []v1.Container {
	return []v1.Container{
		{
			Name:            "redis-connection",
			Image:           "registry.redhat.io/rhscl/redis-32-rhel7",
			Command:         command,
			ImagePullPolicy: v1.PullIfNotPresent,
		},
	}
}
