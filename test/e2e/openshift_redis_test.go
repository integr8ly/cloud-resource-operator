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
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return errorUtil.Wrapf(err, "could not get namespace")
	}

	testRedis := &v1alpha1.Redis{
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
	}

	if err := redisCreateTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "create redis test failure")
	}

	if err := redisDeleteTest(t, f, testRedis, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete redis test failure")
	}

	return nil
}

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

func redisDeleteTest(t *testing.T, f *framework.Framework, testRedis *v1alpha1.Redis, namespace string) error {
	// delete redis resource
	if err := f.Client.Delete(goctx.TODO(), testRedis); err != nil {
		return errorUtil.Wrapf(err, "failed  to delete example redis")
	}

	// check resources have been cleaned up
	rd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rd, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get deployment deletion")
	}

	rpvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rpvc, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get persistent volume claim deletion")
	}

	rs := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redisName,
			Namespace: namespace,
		},
	}
	if err := e2eutil.WaitForDeletion(t, f.Client.Client, rs, retryInterval, timeout); err != nil {
		return errorUtil.Wrapf(err, "could not get service deletion")
	}
	t.Logf("all redis resources have been cleaned")

	return nil
}
