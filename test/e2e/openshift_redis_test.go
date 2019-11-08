package e2e

import (
	goctx "context"
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	appsv1 "k8s.io/api/apps/v1"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OpenshiftRedisBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify redis delete
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// verifies a connection can be made to a postgres resource
func OpenshiftVerifyRedisConnection(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// verify redis connection
	if err := VerifyRedisConnectionTest(t, f, namespace); err != nil {
		return errorUtil.Wrapf(err, "failed to verify connection")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// verify redis delete
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// tests deployment recovery on manual delete of deployment
func OpenshiftVerifyRedisDeploymentRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis deployment
	if err := f.Client.Delete(goctx.TODO(), GetTestDeployment(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis deployment")
	}

	// wait for redis deployment
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test service recovery on manual delete of service
func OpenshiftVerifyRedisServiceRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis service
	if err := f.Client.Delete(goctx.TODO(), GetTestService(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis service")
	}

	// wait for redis service
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test pvc recovery on manual delete of pvc
func OpenshiftVerifyRedisPVCRecovery(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	// delete redis pvc
	if err := f.Client.Delete(goctx.TODO(), GetTestPVC(redisName, namespace)); err != nil {
		return errorUtil.Wrapf(err, "failed to delete redis pvc")
	}

	// wait for redis service
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// test manual updates to redis deployment image, waits to see if cro reconciles and returns image to what is expected
func OpenshiftVerifyRedisDeploymentUpdate(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testRedis, namespace, err := getBasicTestRedis(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get redis")
	}

	// verify redis create
	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	rd := &appsv1.Deployment{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
		return errorUtil.Wrapf(err, "failed to get redis deployment")
	}

	// get wanted container
	wantedContainer := rd.Spec.Template.Spec.Containers[0].Image

	// set unwanted container
	rd.Spec.Template.Spec.Containers[0].Image = "registry.redhat.io/rhscl/redis-5-rhel7"

	// update redis deployment
	if err := f.Client.Update(goctx.TODO(), rd); err != nil {
		return errorUtil.Wrapf(err, "failed to update redis service")
	}

	// wait for redis deployment
	if err := e2eutil.WaitForDeployment(t, f.KubeClient, namespace, redisName, 1, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get redis re-deployment")
	}

	// get updated redis deployment
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
		return errorUtil.Wrapf(err, "failed to get redis deployment")
	}

	// verify the image is what we want
	if rd.Spec.Template.Spec.Containers[0].Image != wantedContainer {
		return errorUtil.New("redis failed to reconcile to wanted image")
	}

	// verify deployment is available
	if err := verifySuccessfulRedisDeploymentStatus(f, namespace); err != nil {
		return errorUtil.Wrapf(err, "unable to verify successful status")
	}

	// cleanup redis
	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

// creates redis resource, verifies everything is as expected
func redisCreateTest(t *testing.T, f *framework.Framework, testRedis *v1alpha1.Redis, namespace string) error {
	// create redis resource
	if err := f.Client.Create(goctx.TODO(), testRedis, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create example redis")
	}
	t.Logf("created %s resource", testRedis.Name)

	// poll cr for complete status phase
	rcr := &v1alpha1.Redis{}
	err := wait.Poll(retryInterval, time.Minute*6, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rcr); err != nil {
			return true, errorUtil.Wrapf(err, "could not get redis cr")
		}
		if rcr.Status.Phase == v1alpha1.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("redis status phase %s", rcr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: rcr.Status.SecretRef.Namespace, Name: rcr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"port", "uri"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %s value not found", k))
		}
	}
	t.Logf("%s secret created successfully", rcr.Status.SecretRef.Name)

	return nil
}

// removes redis resources and verifies all components have been cleaned up
func redisDeleteTest(t *testing.T, f *framework.Framework, testRedis *v1alpha1.Redis, namespace string) error {
	// delete redis resource
	if err := f.Client.Delete(goctx.TODO(), testRedis); err != nil {
		return errorUtil.Wrapf(err, "failed  to delete example redis")
	}

	// check resources have been cleaned up
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestDeployment(redisName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment deletion")
	}

	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestPVC(redisName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get persistent volume claim deletion")
	}

	if err := e2eutil.WaitForDeletion(t, f.Client.Client, GetTestService(redisName, namespace), retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get service deletion")
	}
	t.Logf("all redis resources have been cleaned")

	return nil
}

// verify that the deployment status is available
func verifySuccessfulRedisDeploymentStatus(f *framework.Framework, namespace string) error {
	rd := &appsv1.Deployment{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: redisName}, rd); err != nil {
			return true, errorUtil.Wrapf(err, "could not get redis deployment")
		}
		for _, s := range rd.Status.Conditions {
			if s.Type == appsv1.DeploymentAvailable {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return errorUtil.Wrapf(err, "failed to poll redis deployment")
	}
	return nil
}

func getBasicTestRedis(ctx framework.TestCtx) (*v1alpha1.Redis, string, error) {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return nil, "", errorUtil.Wrapf(err, "could not get namespace")
	}

	return &v1alpha1.Redis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
		Spec: v1alpha1.RedisSpec{
			SecretRef: &v1alpha1.SecretRef{
				Name:      "example-redis-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
		},
	}, namespace, nil
}
