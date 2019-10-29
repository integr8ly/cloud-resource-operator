package e2e

import (
	"testing"
	"time"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis"
	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/test/e2eutil"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	retryInterval = time.Second * 5
	timeout       = time.Second * 30
)

func TestCRO(t *testing.T) {
	redisList := &v1alpha1.Redis{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Redis",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, redisList); err != nil {
		t.Fatalf("failed to add Redis custom resource scheme to framework: %v", err)
	}
	postgresList := &v1alpha1.Postgres{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Postgres",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, postgresList); err != nil {
		t.Fatalf("failed to add Postgres custom resource scheme to framework: %v", err)
	}
	blobstorageList := &v1alpha1.BlobStorage{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BlobStorage",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, blobstorageList); err != nil {
		t.Fatalf("failed to add Blob Storage custom resource scheme to framework: %v", err)
	}
	smtpList := &v1alpha1.SMTPCredentialSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SMTPCredentialSet",
			APIVersion: "integreatly.org/v1alpha1",
		},
	}
	if err := framework.AddToFrameworkScheme(apis.AddToScheme, smtpList); err != nil {
		t.Fatalf("failed to add SMTP Credential Set custom resource scheme to framework: %v", err)
	}
	// run subtests
	t.Run("cro-group", func(t *testing.T) {
		t.Run("Cluster", CROCluster)
	})
}

func CROCluster(t *testing.T) {
	t.Parallel()
	ctx := framework.NewTestCtx(t)
	defer ctx.Cleanup()
	co := framework.CleanupOptions{
		TestContext:   ctx,
		Timeout:       timeout,
		RetryInterval: retryInterval,
	}
	err := ctx.InitializeClusterResources(&co)
	if err != nil {
		t.Fatalf("failed to initialize cluster resources: %v", err)
	}
	t.Log("Initialized cluster resources")
	namespace, err := ctx.GetNamespace()
	if err != nil {
		t.Fatal(err)
	}
	// get global framework variables
	f := framework.Global
	// wait for cloud-resource-operator to be ready
	err = e2eutil.WaitForDeployment(t, f.KubeClient, namespace, "cloud-resource-operator", 1, retryInterval, timeout)
	if err != nil {
		t.Fatal(err)
	}
}
