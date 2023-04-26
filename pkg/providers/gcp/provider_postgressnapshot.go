package gcp

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"github.com/integr8ly/cloud-resource-operator/pkg/annotations"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp/gcpiface"
	errorUtil "github.com/pkg/errors"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ providers.PostgresSnapshotProvider = (*PostgresSnapshotProvider)(nil)
)

const (
	postgresSnapshotProviderName = postgresProviderName + "-snapshots"
	labelLatest                  = "latest"
	labelBucketName              = "bucketName"
	labelObjectName              = "objectName"
)

type PostgresSnapshotProvider struct {
	client            client.Client
	logger            *logrus.Entry
	CredentialManager CredentialManager
	ConfigManager     ConfigManager
}

func NewGCPPostgresSnapshotProvider(client client.Client, logger *logrus.Entry) *PostgresSnapshotProvider {
	return &PostgresSnapshotProvider{
		client:            client,
		logger:            logger.WithFields(logrus.Fields{"provider": postgresProviderName}),
		CredentialManager: NewCredentialMinterCredentialManager(client),
		ConfigManager:     NewDefaultConfigManager(client),
	}
}

func (p *PostgresSnapshotProvider) GetName() string {
	return postgresSnapshotProviderName
}

func (p *PostgresSnapshotProvider) SupportsStrategy(deploymentStrategy string) bool {
	return deploymentStrategy == providers.GCPDeploymentStrategy
}

func (p *PostgresSnapshotProvider) GetReconcileTime(snapshot *v1alpha1.PostgresSnapshot) time.Duration {
	if snapshot.Status.Phase != croType.PhaseComplete {
		return time.Second * 60
	}
	return resources.GetForcedReconcileTimeOrDefault(defaultReconcileTime)
}

func (p *PostgresSnapshotProvider) CreatePostgresSnapshot(ctx context.Context, snap *v1alpha1.PostgresSnapshot, pg *v1alpha1.Postgres) (*providers.PostgresSnapshotInstance, croType.StatusMessage, error) {
	logger := p.logger.WithField("action", "CreatePostgresSnapshot")
	if err := resources.CreateFinalizer(ctx, p.client, snap, DefaultFinalizer); err != nil {
		msg := "failed to set finalizer"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	strategyConfig, err := p.ConfigManager.ReadStorageStrategy(ctx, providers.PostgresResourceType, pg.Spec.Tier)
	if err != nil {
		msg := "failed to retrieve postgres strategy config"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, pg.Namespace)
	if err != nil {
		msg := fmt.Sprintf("failed to reconcile gcp provider credentials for postgres instance %s", pg.Name)
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	storageClient, err := gcpiface.NewStorageAPI(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), logger)
	if err != nil {
		msg := "could not initialise storage client"
		return nil, croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	sqlClient, err := gcpiface.NewSQLAdminService(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), p.logger)
	if err != nil {
		errMsg := "could not initialise sql admin service"
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	return p.reconcilePostgresSnapshot(ctx, snap, pg, strategyConfig, storageClient, sqlClient)
}

func (p *PostgresSnapshotProvider) DeletePostgresSnapshot(ctx context.Context, snap *v1alpha1.PostgresSnapshot, pg *v1alpha1.Postgres) (croType.StatusMessage, error) {
	logger := p.logger.WithField("action", "DeletePostgresSnapshot")
	creds, err := p.CredentialManager.ReconcileProviderCredentials(ctx, pg.Namespace)
	if err != nil {
		msg := fmt.Sprintf("failed to reconcile gcp provider credentials for postgres instance %s", pg.Name)
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	storageClient, err := gcpiface.NewStorageAPI(ctx, option.WithCredentialsJSON(creds.ServiceAccountJson), logger)
	if err != nil {
		msg := "could not initialise storage client"
		return croType.StatusMessage(msg), errorUtil.Wrap(err, msg)
	}
	return p.deletePostgresSnapshot(ctx, snap, storageClient)
}

func (p *PostgresSnapshotProvider) reconcilePostgresSnapshot(ctx context.Context, snap *v1alpha1.PostgresSnapshot, pg *v1alpha1.Postgres, config *StrategyConfig, storageClient gcpiface.StorageAPI, sqlClient gcpiface.SQLAdminService) (*providers.PostgresSnapshotInstance, croType.StatusMessage, error) {
	instanceName := annotations.Get(pg, ResourceIdentifierAnnotation)
	if instanceName == "" {
		errMsg := fmt.Sprintf("failed to find %s annotation for postgres cr %s", ResourceIdentifierAnnotation, pg.Name)
		return nil, croType.StatusMessage(errMsg), errorUtil.New(errMsg)
	}
	snapshotID := strconv.FormatInt(snap.CreationTimestamp.Unix(), 10)
	snap.Status.SnapshotID = snapshotID
	if err := p.client.Status().Update(ctx, snap); err != nil {
		errMsg := fmt.Sprintf("failed to update snapshot %s in namespace %s", snap.Name, snap.Namespace)
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	objectMeta, err := storageClient.GetObjectMetadata(ctx, instanceName, snapshotID)
	if err != nil && err != storage.ErrObjectNotExist {
		errMsg := fmt.Sprintf("failed to retrieve object metadata for bucket %s and object %s", instanceName, snapshotID)
		return nil, croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	if objectMeta == nil {
		statusMessage, err := p.createPostgresSnapshot(ctx, snap, pg, config, storageClient, sqlClient)
		return nil, statusMessage, err
	}
	statusMessage, err := p.reconcilePostgresSnapshotLabels(ctx, snap, objectMeta, storageClient)
	if err != nil {
		return nil, statusMessage, err
	}
	return &providers.PostgresSnapshotInstance{
		Name: objectMeta.Name,
	}, statusMessage, nil
}

func (p *PostgresSnapshotProvider) createPostgresSnapshot(ctx context.Context, snap *v1alpha1.PostgresSnapshot, pg *v1alpha1.Postgres, config *StrategyConfig, storageClient gcpiface.StorageAPI, sqlClient gcpiface.SQLAdminService) (croType.StatusMessage, error) {
	instanceName := annotations.Get(pg, ResourceIdentifierAnnotation)
	if pg.Status.Phase == croType.PhaseInProgress {
		errMsg := fmt.Sprintf("waiting for postgres instance %s to be available before creating a snapshot", instanceName)
		return croType.StatusMessage(errMsg), errorUtil.New(errMsg)
	}
	if pg.Status.Phase == croType.PhaseDeleteInProgress {
		errMsg := "cannot create snapshot of postgres instance when deletion is in progress"
		return croType.StatusMessage(errMsg), errorUtil.New(errMsg)
	}
	err := storageClient.CreateBucket(ctx, instanceName, config.ProjectID, &storage.BucketAttrs{
		Location: config.Region,
	})
	if err != nil && !resources.IsConflictError(err) {
		errMsg := fmt.Sprintf("failed to create bucket with name %s", instanceName)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	instance, err := sqlClient.GetInstance(ctx, config.ProjectID, instanceName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to find postgres instance with name %s", instanceName)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	serviceAccount := fmt.Sprintf("serviceAccount:%s", instance.ServiceAccountEmailAddress)
	err = storageClient.SetBucketPolicy(ctx, instanceName, serviceAccount, "roles/storage.objectAdmin")
	if err != nil {
		errMsg := fmt.Sprintf("failed to set policy on bucket %s", instanceName)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	_, err = sqlClient.ExportDatabase(ctx, config.ProjectID, instanceName, &sqladmin.InstancesExportRequest{
		ExportContext: &sqladmin.ExportContext{
			Databases: []string{"postgres"},
			FileType:  "SQL",
			Uri:       fmt.Sprintf("gs://%s/%s", instanceName, snap.Status.SnapshotID),
		},
	})
	if err != nil && !resources.IsConflictError(err) {
		errMsg := fmt.Sprintf("failed to export database from postgres instance %s", instanceName)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	msg := fmt.Sprintf("snapshot creation started for %s", snap.Name)
	return croType.StatusMessage(msg), nil
}

func (p *PostgresSnapshotProvider) reconcilePostgresSnapshotLabels(ctx context.Context, snap *v1alpha1.PostgresSnapshot, objectMeta *storage.ObjectAttrs, storageClient gcpiface.StorageAPI) (croType.StatusMessage, error) {
	if !resources.HasLabel(snap, labelBucketName) {
		resources.AddLabel(snap, labelBucketName, objectMeta.Bucket)
	}
	if !resources.HasLabel(snap, labelObjectName) {
		resources.AddLabel(snap, labelObjectName, objectMeta.Name)
	}
	if err := p.client.Update(ctx, snap); err != nil {
		errMsg := fmt.Sprintf("failed to update postgres snapshot cr %s", snap.Name)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	latestSnapshotCR, _, err := getLatestPostgresSnapshotCR(ctx, objectMeta.Bucket, snap.Namespace, p.client, storageClient)
	if err != nil {
		errMsg := fmt.Sprintf("failed to determine latest snapshot id for instance %s", objectMeta.Bucket)
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	if objectMeta.Name == latestSnapshotCR.Status.SnapshotID {
		if !resources.HasLabel(snap, labelLatest) {
			resources.AddLabel(snap, labelLatest, objectMeta.Bucket)
		}
		snap.Spec.SkipDelete = true
		if err = p.client.Update(ctx, snap); err != nil {
			errMsg := fmt.Sprintf("failed to update postgres snapshot cr %s", snap.Name)
			return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}
	}
	labelSelector := labels.NewSelector()
	matchBucketLabel, err := labels.NewRequirement(labelBucketName, selection.Equals, []string{objectMeta.Bucket})
	if err != nil {
		errMsg := "failed to build label requirement"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	matchObjectLabel, err := labels.NewRequirement(labelObjectName, selection.Equals, []string{objectMeta.Name})
	if err != nil {
		errMsg := "failed to build label requirement"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	labelSelector.Add(*matchBucketLabel, *matchObjectLabel)
	fieldSelector, err := fields.ParseSelector(fmt.Sprintf("metadata.name!=%s", latestSnapshotCR.Name))
	if err != nil {
		errMsg := "failed to parse field selector"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	snapshots := &v1alpha1.PostgresSnapshotList{}
	err = p.client.List(ctx, snapshots, &client.ListOptions{
		Namespace:     snap.Namespace,
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
	if err != nil {
		errMsg := "failed to list postgres snapshots"
		return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
	}
	for i := range snapshots.Items {
		item := &snapshots.Items[i]
		resources.RemoveLabel(item, labelLatest)
		item.Spec.SkipDelete = false
		if err = p.client.Update(ctx, item); err != nil {
			errMsg := fmt.Sprintf("failed to remove label %q from postgres snapshot cr", labelLatest)
			return croType.StatusMessage(errMsg), errorUtil.Wrap(err, errMsg)
		}
	}
	msg := fmt.Sprintf("snapshot %s successfully reconciled", snap.Name)
	return croType.StatusMessage(msg), nil
}

func (p *PostgresSnapshotProvider) deletePostgresSnapshot(ctx context.Context, snap *v1alpha1.PostgresSnapshot, storageClient gcpiface.StorageAPI) (croType.StatusMessage, error) {
	if !snap.Spec.SkipDelete {
		bucketName := resources.GetLabel(snap, labelBucketName)
		if bucketName == "" {
			errMsg := fmt.Sprintf("failed to find %q label for postgres snapshot cr %s", labelBucketName, snap.Name)
			return croType.StatusMessage(errMsg), errorUtil.New(errMsg)
		}
		objectName := resources.GetLabel(snap, labelObjectName)
		if objectName == "" {
			errMsg := fmt.Sprintf("failed to find %q label for postgres snapshot cr %s", labelObjectName, snap.Name)
			return croType.StatusMessage(errMsg), errorUtil.New(errMsg)
		}
		if !resources.HasLabelWithValue(snap, labelLatest, bucketName) {
			err := storageClient.DeleteObject(ctx, bucketName, objectName)
			if err != nil {
				errMsg := fmt.Sprintf("failed to delete snapshot %s from bucket %s", objectName, bucketName)
				return croType.StatusMessage(errMsg), err
			}
			objects, err := storageClient.ListObjects(ctx, bucketName, nil)
			if err != nil {
				errMsg := fmt.Sprintf("failed to list objects from bucket %s", bucketName)
				return croType.StatusMessage(errMsg), err
			}
			if len(objects) == 0 {
				err = storageClient.DeleteBucket(ctx, bucketName)
				if err != nil {
					errMsg := fmt.Sprintf("failed to delete bucket %s", bucketName)
					return croType.StatusMessage(errMsg), err
				}
			}
		}
	}
	resources.RemoveFinalizer(&snap.ObjectMeta, DefaultFinalizer)
	if err := p.client.Update(ctx, snap); err != nil {
		errMsg := "failed to update snapshot as part of finalizer reconcile"
		return croType.StatusMessage(errMsg), errorUtil.Wrapf(err, errMsg)
	}
	msg := fmt.Sprintf("snapshot %s deleted", snap.Name)
	return croType.StatusMessage(msg), nil
}

func getLatestPostgresSnapshotCR(ctx context.Context, instanceName string, namespace string, k8sClient client.Client, storageClient gcpiface.StorageAPI) (*v1alpha1.PostgresSnapshot, int, error) {
	snapshots := &v1alpha1.PostgresSnapshotList{}
	err := k8sClient.List(ctx, snapshots, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		return nil, 0, err
	}
	if len(snapshots.Items) == 0 {
		return nil, 0, fmt.Errorf("failed to find matching postgres snapshot cr for bucket %s", instanceName)
	}
	objects, err := storageClient.ListObjects(ctx, instanceName, nil)
	if err != nil {
		if err == storage.ErrBucketNotExist {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	inProgress := 0
	for i := range snapshots.Items {
		if snapshots.Items[i].Status.Phase == croType.PhaseInProgress {
			inProgress++
		}
	}
	if len(objects) == 0 && (inProgress == len(snapshots.Items)) {
		return nil, 0, nil
	} else if len(objects) == 0 {
		return nil, 0, fmt.Errorf("could not find any objects in bucket %q", instanceName)
	}
	var errs []error
	sort.Slice(objects, func(i, j int) bool {
		a, err := strconv.ParseInt(objects[i].Name, 10, 64)
		if err != nil {
			errs = append(errs, err)
		}
		b, err := strconv.ParseInt(objects[j].Name, 10, 64)
		if err != nil {
			errs = append(errs, err)
		}
		return time.Unix(a, 0).After(time.Unix(b, 0))
	})
	if len(errs) > 0 {
		return nil, 0, fmt.Errorf("failure while sorting objects: %v", errs)
	}
	labelSelector := labels.NewSelector()
	matchBucketLabel, err := labels.NewRequirement(labelBucketName, selection.Equals, []string{instanceName})
	if err != nil {
		errMsg := "failed to build label requirement"
		return nil, 0, errorUtil.Wrap(err, errMsg)
	}
	matchObjectLabel, err := labels.NewRequirement(labelObjectName, selection.Equals, []string{objects[0].Name})
	if err != nil {
		errMsg := "failed to build label requirement"
		return nil, 0, errorUtil.Wrap(err, errMsg)
	}
	labelSelector = labelSelector.Add(*matchBucketLabel, *matchObjectLabel)
	snapshots = &v1alpha1.PostgresSnapshotList{}
	err = k8sClient.List(ctx, snapshots, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, 0, err
	}
	if len(snapshots.Items) == 0 {
		return nil, 0, fmt.Errorf("failed to find matching postgres snapshot cr for object %s from bucket %s", objects[0].Name, instanceName)
	}
	if len(snapshots.Items) > 1 {
		return nil, 0, fmt.Errorf("expected to find exactly one postgres snapshot with label selector %q, but found %d", labelSelector.String(), len(snapshots.Items))
	}
	return &snapshots.Items[0], len(objects), nil
}

func getAllSnapshotsForInstance(ctx context.Context, k8sClient client.Client, pg *v1alpha1.Postgres) ([]v1alpha1.PostgresSnapshot, error) {
	labelSelector := labels.NewSelector()
	matchBucketLabel, err := labels.NewRequirement(labelBucketName, selection.Equals, []string{annotations.Get(pg, ResourceIdentifierAnnotation)})
	if err != nil {
		errMsg := "failed to build label requirement"
		return nil, errorUtil.Wrap(err, errMsg)
	}
	labelSelector.Add(*matchBucketLabel)
	snapshotsList := &v1alpha1.PostgresSnapshotList{}
	err = k8sClient.List(ctx, snapshotsList, &client.ListOptions{
		Namespace:     pg.Namespace,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	snapshots := snapshotsList.Items
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreationTimestamp.Before(&snapshots[j].CreationTimestamp)
	})
	return snapshots, nil
}
