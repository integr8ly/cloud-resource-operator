package e2e

import (
	goctx "context"
	"database/sql"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/util/wait"

	bv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/apis/integreatly/v1alpha1"

	framework "github.com/operator-framework/operator-sdk/pkg/test"

	_ "github.com/lib/pq"
	errorUtil "github.com/pkg/errors"
)

const (
	permissionJobName = "permission-job"
	connectionJobName = "connection-job"
)

// verify a connection can be made to postgres instance
func VerifyPostgresConnectionTest(t *testing.T, f *framework.Framework, namespace string) error {
	// get postgres secret
	sec, err := getPostgresSecret(t, f, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// create psql connection command
	pgp := "PGPASSWORD=" + string(sec.Data["password"])
	psqlConn := fmt.Sprintf("psql --username=%s --host=%s --port=%s --dbname=%s",
		sec.Data["username"], sec.Data["host"], string(sec.Data["port"]), sec.Data["database"])
	psqlCommand := []string{"/bin/sh", "-i", "-c", pgp, psqlConn}

	// create postgres connection job
	pj := postgresConnectionJob(connectionJobName, namespace, psqlCommand)
	if err := f.Client.Create(goctx.TODO(), pj, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create postgres connection job")
	}

	// poll postgres connection job for success
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: connectionJobName}, pj); err != nil {
			return true, errorUtil.Wrapf(err, "could not get connection job")
		}
		for _, s := range pj.Status.Conditions {
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

func VerifyPostgresPermissionTest(t *testing.T, f *framework.Framework, namespace string) error {
	// get postgres secret
	sec, err := getPostgresSecret(t, f, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// create psql execute command
	pgp := "PGPASSWORD=" + string(sec.Data["password"])
	psqlConn := fmt.Sprintf("psql --username=%s --host=%s --port=%s --dbname=%s --command='CREATE TABLE test_table (column1 text, column2 text);'",
		sec.Data["username"], sec.Data["host"], string(sec.Data["port"]), sec.Data["database"])
	psqlCommand := []string{"/bin/sh", "-i", "-c", pgp, psqlConn}

	// create postgres connection job
	pj := postgresConnectionJob(permissionJobName, namespace, psqlCommand)
	if err := f.Client.Create(goctx.TODO(), pj, getCleanupOptions(t)); err != nil {
		return errorUtil.Wrapf(err, "could not create postgres connection job")
	}

	// poll postgres command execution job to succeed
	err = wait.Poll(retryInterval, timeout, func() (done bool, err error) {
		if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: permissionJobName}, pj); err != nil {
			return true, errorUtil.Wrapf(err, "could not get connection job")
		}
		for _, s := range pj.Status.Conditions {
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

// verify postgres connection secret is valid
func VerifyPostgresSecretTest(t *testing.T, f *framework.Framework, namespace string) error {
	sec, err := getPostgresSecret(t, f, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// get connection details from secret
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		sec.Data["host"], string(sec.Data["port"]), sec.Data["username"], sec.Data["password"], sec.Data["database"])

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return errorUtil.Wrapf(err, "could not verify postgres connection string")
	}
	t.Logf("successfully verified %s", psqlInfo)

	if err := db.Close(); err != nil {
		return errorUtil.Wrapf(err, "failed to close postgres connection")
	}

	return nil
}

func getPostgresSecret(t *testing.T, f *framework.Framework, namespace string) (v1.Secret, error) {
	sec := v1.Secret{}
	pcr := &v1alpha1.Postgres{}
	// poll cr for complete status phase
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pcr); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get resource")
	}
	t.Logf("postgres status phase %s", pcr.Status.Phase)

	// get created secret
	if err := f.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: pcr.Status.SecretRef.Namespace, Name: pcr.Status.SecretRef.Name}, &sec); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get secret")
	}

	return sec, nil
}

// returns job template
func postgresConnectionJob(jobName string, namespace string, containerCommand []string) *bv1.Job {
	return &bv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
		},
		Spec: bv1.JobSpec{
			Parallelism:           int32Ptr(1),
			Completions:           int32Ptr(1),
			ActiveDeadlineSeconds: int64Ptr(300),
			BackoffLimit:          int32Ptr(1),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "postgres-connection",
				},
				Spec: v1.PodSpec{
					Containers:    buildPostgresContainer(containerCommand),
					RestartPolicy: v1.RestartPolicyOnFailure,
				},
			},
		},
	}
}

func buildPostgresContainer(command []string) []v1.Container {
	return []v1.Container{
		{
			Name:    "postgres-connection",
			Image:   "registry.redhat.io/rhscl/postgresql-96-rhel7",
			Command: command,
			Ports: []v1.ContainerPort{
				{
					ContainerPort: int32(5436),
					Protocol:      v1.ProtocolTCP,
				},
			},
			ImagePullPolicy: v1.PullIfNotPresent,
		},
	}
}

func int32Ptr(i int32) *int32 { return &i }

func int64Ptr(i int64) *int64 { return &i }
