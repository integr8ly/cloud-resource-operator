package e2e

import (
	goctx "context"
	"fmt"
	"testing"

	t1 "github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1/types"

	"k8s.io/apimachinery/pkg/api/errors"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"

	errorUtil "github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func OpenshiftSMTPBasicTest(t *testing.T, f *framework.Framework, ctx framework.TestCtx) error {
	testSMTP, namespace, err := getBasicSMTP(ctx)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get smtp")
	}

	// verify smtp create
	if err := smtpCreateTest(t, f, testSMTP, namespace); err != nil {
		return errorUtil.Wrapf(err, "create smtp test failure")
	}

	// verify smtp delete
	if err := smtpDeleteTest(t, f, testSMTP, namespace); err != nil {
		return errorUtil.Wrapf(err, "delete smtp test failure")
	}

	t.Logf("smtp basic test pass")
	return nil
}

// creates smtp resource, verifies everything is as expected
func smtpCreateTest(t *testing.T, f *framework.Framework, testSMTP *v1alpha1.SMTPCredentialSet, namespace string) error {
	// create smtp resource
	if err := f.Client.Create(goctx.TODO(), testSMTP, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create example smtp")
	}
	t.Logf("created %s resource", testSMTP.Name)

	// poll cr for complete status phase
	scr := &v1alpha1.SMTPCredentialSet{}
	err := wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: smtpName}, scr); err != nil {
			return true, errorUtil.Wrapf(err, "could not get smtp cr")
		}
		if scr.Status.Phase == t1.StatusPhase("complete") {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	t.Logf("smtp status phase %s", scr.Status.Phase)

	// get created secret
	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: scr.Status.SecretRef.Namespace, Name: scr.Status.SecretRef.Name}, &sec); err != nil {
		return errorUtil.Wrapf(err, "could not get secret")
	}

	// check for expected key values
	for _, k := range []string{"username", "password", "port", "host", "tls"} {
		if sec.Data[k] == nil {
			return errorUtil.New(fmt.Sprintf("secret %s value not found", k))
		}
	}

	t.Logf("%s secret created successfully", scr.Status.SecretRef.Name)
	return nil
}

// removes smtp resource and verifies all components have been cleaned up
func smtpDeleteTest(t *testing.T, f *framework.Framework, testSMTP *v1alpha1.SMTPCredentialSet, namespace string) error {
	// delete smtp resource
	if err := f.Client.Delete(goctx.TODO(), testSMTP); err != nil {
		return errorUtil.Wrapf(err, "failed to delete example smtp")
	}
	t.Logf("%s custom resource deleted", testSMTP.Name)

	sec := v1.Secret{}
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: testSMTP.Spec.SecretRef.Name}, &sec); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return errorUtil.Wrapf(err, "could not get secret")
	}
	t.Logf("")
	return nil
}

func getBasicSMTP(ctx framework.TestCtx) (*v1alpha1.SMTPCredentialSet, string, error) {
	namespace, err := ctx.GetNamespace()
	if err != nil {
		return nil, "", errorUtil.Wrapf(err, "could not get namespace")
	}

	return &v1alpha1.SMTPCredentialSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      smtpName,
			Namespace: namespace,
		},
		Spec: v1alpha1.SMTPCredentialSetSpec{
			SecretRef: &t1.SecretRef{
				Name:      "example-smtp-sec",
				Namespace: namespace,
			},
			Tier: "development",
			Type: "workshop",
			Env: &t1.Env{
				Name:  "TEST",
				Value: "test",
			},
			Labels: &t1.Labels{
				ProductName: "testproduct",
			},
		},
	}, namespace, nil
}
