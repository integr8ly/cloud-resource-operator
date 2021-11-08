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

package postgres

import (
	"context"
	"fmt"

	croType "github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1/types"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	"github.com/integr8ly/cloud-resource-operator/pkg/resources"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers/openshift"

	"github.com/integr8ly/cloud-resource-operator/pkg/providers"
	"github.com/sirupsen/logrus"
	"time"

	"github.com/integr8ly/cloud-resource-operator/apis/integreatly/v1alpha1"
	errorUtil "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

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

var log = logf.Log.WithName("controller_postgres")

// PostgresReconciler reconciles a Postgres object
type PostgresReconciler struct {
	k8sclient.Client
	scheme           *runtime.Scheme
	logger           *logrus.Entry
	resourceProvider *resources.ReconcileResourceProvider
	providerList     []providers.PostgresProvider
}

// New returns a new reconcile.Reconciler
func New(mgr manager.Manager) (*PostgresReconciler, error) {
	restConfig := controllerruntime.GetConfigOrDie()
	restConfig.Timeout = time.Second * 10

	client, err := k8sclient.New(restConfig, k8sclient.Options{
		Scheme: mgr.GetScheme(),
	})
	if err != nil {
		return nil, err
	}

	clientSet, err := resources.GetK8Client()
	if err != nil {
		return nil, errorUtil.Wrap(err, "failed to build client set")
	}

	logger := logrus.WithFields(logrus.Fields{"controller": "controller_postgres"})
	providerList := []providers.PostgresProvider{openshift.NewOpenShiftPostgresProvider(client, clientSet, logger), aws.NewAWSPostgresProvider(client, logger)}
	rp := resources.NewResourceProvider(client, mgr.GetScheme(), logger)
	return &PostgresReconciler{
		Client:           client,
		scheme:           mgr.GetScheme(),
		logger:           logger,
		resourceProvider: rp,
		providerList:     providerList,
	}, nil
}

func (r *PostgresReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&integreatlyv1alpha1.Postgres{}).
		Watches(&source.Kind{Type: &v1alpha1.Postgres{}}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &v1alpha1.Postgres{},
		}).
		Complete(r)
}

// ClusterRole permissions

// +kubebuilder:rbac:groups="config.openshift.io",resources=infrastructures;networks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumes;configmaps,verbs="*"
// +kubebuilder:rbac:groups="monitoring.coreos.com",resources=prometheusrules,verbs="*"

// Role permissions

// +kubebuilder:rbac:groups="",resources=pods;pods/exec;services;services/finalizers;endpoints;persistentvolumeclaims;events;configmaps;secrets,verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="apps",resources="*",verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="monitoring.coreos.com",resources=servicemonitors,verbs=get;create,namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="monitoring.coreos.com",resources=prometheusrules,verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="cloud-resource-operator",resources=deployments/finalizers,verbs=update,namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="integreatly",resources="*",verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups=integreatly.org,resources="*";smtpcredentialset;redis;postgres;redissnapshots;postgressnapshots,verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups=integreatly.org,resources=blobstorages/status,verbs=get;update;patch,namespace=cloud-resource-operator
// +kubebuilder:rbac:groups=integreatly.org,resources=blobstorages,verbs=get;list;watch;create;update;patch;delete,namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="config.openshift.io",resources="*";infrastructures;schedulers;featuregates;networks;ingresses;clusteroperators;authentications;builds,verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups="cloudcredential.openshift.io",resources=credentialsrequests,verbs="*",namespace=cloud-resource-operator
// +kubebuilder:rbac:groups=operators.coreos.com,resources=catalogsources,verbs=get;update;patch,namespace=cloud-resource-operator

func (r *PostgresReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	r.logger.Info("reconciling Postgres")
	cfgMgr := providers.NewConfigManager(providers.DefaultProviderConfigMapName, request.Namespace, r.Client)

	// Fetch the Postgres instance
	instance := &v1alpha1.Postgres{}
	err := r.Client.Get(context.TODO(), request.NamespacedName, instance)
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
		return ctrl.Result{}, errorUtil.Wrapf(err, "failed to read deployment type config for deployment %s", instance.Spec.Type)
	}

	// Check the CR for existing Strategy
	strategyToUse := stratMap.Postgres
	if instance.Status.Strategy != "" {
		strategyToUse = instance.Status.Strategy
		if strategyToUse != stratMap.Postgres {
			r.logger.Infof("strategy and provider already set, changing of cloud-resource-config config maps not allowed in existing installation. the existing strategy is '%s' , cloud-resource-config is now set to '%s'. operator will continue to use existing strategy", strategyToUse, stratMap.Postgres)
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

		// delete the postgres if the deletion timestamp exists
		if instance.DeletionTimestamp != nil {
			msg, err := p.DeletePostgres(ctx, instance)
			if err != nil {
				if updateErr := resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
					return ctrl.Result{}, updateErr
				}
				return ctrl.Result{}, errorUtil.Wrapf(err, "failed to perform provider-specific storage deletion")
			}

			r.logger.Info("waiting on Postgres to successfully delete")
			if err = resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseDeleteInProgress, msg); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
		}

		// handle skip create
		if instance.Spec.SkipCreate {
			r.logger.Info("skipCreate found, skipping postgres reconcile")
			if err := resources.UpdatePhase(ctx, r.Client, instance, croType.PhasePaused, croType.StatusSkipCreate); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
		}

		// create the postgres instance
		ps, msg, err := p.ReconcilePostgres(ctx, instance)
		if err != nil {
			instance.Status.SecretRef = &croType.SecretRef{}
			if updateErr := resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseFailed, msg.WrapError(err)); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
		if ps == nil {
			r.logger.Info(msg)
			instance.Status.SecretRef = &croType.SecretRef{}
			if err = resources.UpdatePhase(ctx, r.Client, instance, croType.PhaseInProgress, msg); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: p.GetReconcileTime(instance)}, nil
		}

		// return the connection secret
		if err := r.resourceProvider.ReconcileResultSecret(ctx, instance, ps.DeploymentDetails.Data()); err != nil {
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
	return ctrl.Result{}, errorUtil.New(fmt.Sprintf("unsupported deployment strategy %s", stratMap.Postgres))
}
