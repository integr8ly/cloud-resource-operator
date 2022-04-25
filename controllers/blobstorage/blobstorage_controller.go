/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package blobstorage

import (
	"context"
	"fmt"
	"time"

	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers/openshift"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	"github.com/sirupsen/logrus"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	errorUtil "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	integreatlyv1alpha1 "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
)

var log = logf.Log.WithName("controller_blobstorage")

// BlobStorageReconciler reconciles a BlobStorage object
type BlobStorageReconciler struct {
	k8sclient.Client
	scheme           *runtime.Scheme
	logger           *logrus.Entry
	resourceProvider *resources.ReconcileResourceProvider
	providerList     []providers.BlobStorageProvider
}

// New returns a new reconcile.Reconciler
func New(mgr manager.Manager) (*BlobStorageReconciler, error) {
	restConfig := controllerruntime.GetConfigOrDie()
	restConfig.Timeout = time.Second * 10
	client, err := k8sclient.New(restConfig, k8sclient.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return nil, err
	}

	logger := logrus.WithFields(logrus.Fields{"controller": "controller_blobstorage"})
	providerList := []providers.BlobStorageProvider{aws.NewAWSBlobStorageProvider(client, logger), openshift.NewBlobStorageProvider(client, logger)}
	rp := resources.NewResourceProvider(client, mgr.GetScheme(), logger)
	return &BlobStorageReconciler{
		Client:           client,
		scheme:           mgr.GetScheme(),
		logger:           logger,
		resourceProvider: rp,
		providerList:     providerList,
	}, nil
}

func (r *BlobStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&integreatlyv1alpha1.BlobStorage{}).
		Watches(&source.Kind{Type: &v1alpha1.BlobStorage{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}

func (r *BlobStorageReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("reconciling BlobStorage")
	ctx := context.TODO()
	cfgMgr := providers.NewConfigManager(providers.DefaultProviderConfigMapName, request.Namespace, r.Client)

	// Fetch the BlobStorage instance
	instance := &v1alpha1.BlobStorage{}
	err := r.Client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	stratMap, err := cfgMgr.GetStrategyMappingForDeploymentType(ctx, instance.Spec.Type)
	if err != nil {
		if updateErr := resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, croType.StatusDeploymentConfigNotFound.WrapError(err)); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Check the CR for existing Strategy
	strategyToUse := stratMap.BlobStorage
	if instance.Status.Strategy != "" {
		strategyToUse = instance.Status.Strategy
		if strategyToUse != stratMap.BlobStorage {
			r.logger.Infof("strategy and provider already set, changing of cloud-resource-config config maps not allowed in existing installation. the existing strategy is '%s' , cloud-resource-config is now set to '%s'. operator will continue to use existing strategy", strategyToUse, stratMap.BlobStorage)
		}
	}

	for _, p := range r.providerList {
		if !p.SupportsStrategy(strategyToUse) {
			continue
		}
		instance.Status.Strategy = strategyToUse
		instance.Status.Provider = p.GetName()
		if instance.Status.Strategy != strategyToUse || instance.Status.Provider != p.GetName() {
			if err = r.Client.Status().Update(ctx, instance); err != nil {
				return ctrl.Result{}, errorUtil.Wrapf(err, "failed to update instance %s in namespace %s", instance.Name, instance.Namespace)
			}
		}

		if instance.GetDeletionTimestamp() != nil {
			msg, err := p.DeleteStorage(ctx, instance)
			if err != nil {
				if updateErr := resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, errorUtil.Wrapf(err, "failed to perform provider-specific storage deletion")
			}

			r.logger.Info("waiting on blob storage to successfully delete")
			if err = resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseDeleteInProgress, msg.WrapError(err)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
		}

		bsi, msg, err := p.CreateStorage(ctx, instance)
		if err != nil {
			instance.Status.SecretRef = &croType.SecretRef{}
			if updateErr := resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
		if bsi == nil {
			r.logger.Info("secret data is still reconciling, blob storage is nil")
			instance.Status.SecretRef = &croType.SecretRef{}
			if err = resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseInProgress, msg); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
		}

		if err := r.resourceProvider.ReconcileResultSecret(ctx, instance, bsi.DeploymentDetails.Data()); err != nil {
			return ctrl.Result{}, errorUtil.Wrap(err, "failed to reconcile secret")
		}

		instance.Status.Phase = croType.PhaseComplete
		instance.Status.Message = msg
		instance.Status.SecretRef = instance.Spec.SecretRef
		instance.Status.Strategy = strategyToUse
		instance.Status.Provider = p.GetName()
		if err = r.Client.Status().Update(ctx, instance); err != nil {
			return ctrl.Result{}, errorUtil.Wrapf(err, "failed to update instance %s in namespace %s", instance.Name, instance.Namespace)
		}
		return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
	}

	// unsupported strategy
	if err = resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, croType.StatusUnsupportedType.WrapError(err)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, errorUtil.New(fmt.Sprintf("unsupported deployment strategy %s", stratMap.BlobStorage))
}
