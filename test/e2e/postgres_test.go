package e2e

import (
	goctx "context"
	"database/sql"
	"fmt"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/util/wait"

	bv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"

	_ "github.com/lib/pq"
	errorUtil "github.com/pkg/errors"
)

const (
	postgresPermissionJobName = "postgres-permission-job"
	postgresConnectionJobName = "postgres-connection-job"
)

// verify a connection can be made to postgres instance
func VerifyPostgresConnectionTest(t TestingTB, ctx *TestingContext, namespace string) error {
	// get postgres secret
	sec, err := getPostgresSecret(t, ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// create psql connection command
	psqlCommand := []string{
		"/bin/sh",
		"-i",
		"-c",
		"PGPASSWORD=" + string(sec.Data["password"]),
		fmt.Sprintf("psql --username=%s --host=%s --port=%s --dbname=%s",
			sec.Data["username"], sec.Data["host"], string(sec.Data["port"]), sec.Data["database"])}

	// create postgres connection job
	pj := ConnectionJob(buildPostgresContainer(psqlCommand), postgresConnectionJobName, namespace)
	if err := ctx.Client.Create(goctx.TODO(), pj); err != nil {
		return errorUtil.Wrapf(err, "could not create postgres connection job")
	}

	// poll postgres connection job for success
	err = wait.PollUntilContextTimeout(context.TODO(), retryInterval, timeout, false, func(ctx2 context.Context) (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresConnectionJobName}, pj); err != nil {
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

func VerifyPostgresPermissionTest(t TestingTB, ctx *TestingContext, namespace string) error {
	// get postgres secret
	sec, err := getPostgresSecret(t, ctx, namespace)
	if err != nil {
		return errorUtil.Wrapf(err, "failed to get secret")
	}

	// create psql execute command
	pgp := "PGPASSWORD=" + string(sec.Data["password"])
	psqlConn := fmt.Sprintf("psql --username=%s --host=%s --port=%s --dbname=%s --command='CREATE TABLE test_table (column1 text, column2 text);'",
		sec.Data["username"], sec.Data["host"], string(sec.Data["port"]), sec.Data["database"])
	psqlCommand := []string{"/bin/sh", "-i", "-c", pgp, psqlConn}

	// create postgres connection job
	pj := ConnectionJob(buildPostgresContainer(psqlCommand), postgresPermissionJobName, namespace)
	if err := ctx.Client.Create(goctx.TODO(), pj); err != nil {
		return errorUtil.Wrapf(err, "could not create postgres connection job")
	}

	// poll postgres command execution job to succeed
	err = wait.PollUntilContextTimeout(context.TODO(), retryInterval, timeout, false, func(ctx2 context.Context) (done bool, err error) {
		if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresPermissionJobName}, pj); err != nil {
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
func VerifyPostgresSecretTest(t TestingTB, ctx *TestingContext, namespace string) error {
	sec, err := getPostgresSecret(t, ctx, namespace)
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

func getPostgresSecret(t TestingTB, ctx *TestingContext, namespace string) (v1.Secret, error) {
	sec := v1.Secret{}
	pcr := &v1alpha1.Postgres{}
	// poll cr for complete status phase
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: namespace, Name: postgresName}, pcr); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get resource")
	}
	t.Logf("postgres status phase %s", pcr.Status.Phase)

	// get created secret
	if err := ctx.Client.Get(goctx.TODO(), types.NamespacedName{Namespace: pcr.Status.SecretRef.Namespace, Name: pcr.Status.SecretRef.Name}, &sec); err != nil {
		return sec, errorUtil.Wrapf(err, "could not get secret")
	}

	return sec, nil
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
