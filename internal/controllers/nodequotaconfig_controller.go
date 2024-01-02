/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"

	danav1 "github.com/dana-team/hns/api/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	danav1alpha1 "github.com/dana-team/hns-nqs-plugin/api/v1alpha1"
	"github.com/dana-team/hns-nqs-plugin/internal/utils"
)

// NodeQuotaConfigReconciler reconciles a NodeQuotaConfig object
type NodeQuotaConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dana.hns.io,resources=nodequotaconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups="dana.hns.io",resources=subnamespaces,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=dana.hns.io,resources=nodequotaconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dana.hns.io,resources=nodequotaconfigs/finalizers,verbs=update

func (r *NodeQuotaConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	config := danav1alpha1.NodeQuotaConfig{}
	oldConfigStatus := config.Status.DeepCopy()
	if err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, &config); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Start calculating resources")
	requeue, err := r.CalculateRootSubnamespaces(ctx, &config, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	utils.DeleteExpiredReservedResources(&config, logger)
	if err := r.UpdateConfigStatus(ctx, config, *oldConfigStatus, logger); err != nil {
		return ctrl.Result{}, err
	}

	if requeue {
		return ctrl.Result{RequeueAfter: time.Duration(config.Spec.ReservedHoursToLive) * time.Hour}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeQuotaConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&danav1alpha1.NodeQuotaConfig{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.requestConfigReconcile),
		).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

// CalculateRootSubnamespaces calculates the resource allocation for the root subnamespaces based on the provided NodeQuotaConfig.
// It takes a context, the NodeQuotaConfig to reconcile, and a logger for logging informational messages.
// It returns an error (if any occurred) during the calculation.
func (r *NodeQuotaConfigReconciler) CalculateRootSubnamespaces(ctx context.Context, config *danav1alpha1.NodeQuotaConfig, logger logr.Logger) (bool, error) {
	requeue := false
	for _, rootSubnamespace := range config.Spec.Roots {
		logger.Info(fmt.Sprintf("Starting to calculate RootSubnamespace %s", rootSubnamespace.RootNamespace))
		rootResources := v1.ResourceList{}
		var processedSecondaryRoots []danav1.Subnamespace

		for _, secondaryRoot := range rootSubnamespace.SecondaryRoots {
			logger.Info(fmt.Sprintf("Starting to calculate Secondary root %s", secondaryRoot.Name))
			secondaryRootSns, secondaryRequeue, err := utils.ProcessSecondaryRoot(ctx, r.Client, secondaryRoot, config, rootSubnamespace.RootNamespace, logger)
			if err != nil {
				return false, err
			}

			// it's enough that one secondaryRoot signals a requeue
			if secondaryRequeue {
				requeue = true
			}

			processedSecondaryRoots = append(processedSecondaryRoots, secondaryRootSns)
			rootResources = utils.MergeTwoResourceList(secondaryRootSns.Spec.ResourceQuotaSpec.Hard, rootResources)
		}
		if err := utils.UpdateRootSubnamespace(ctx, rootResources, rootSubnamespace, logger, r.Client); err != nil {
			return false, err
		}
		if err := utils.UpdateProcessedSecondaryRoots(ctx, processedSecondaryRoots, logger, r.Client); err != nil {
			return false, err
		}
	}
	return requeue, nil
}

// UpdateConfigStatus updates the status of the NodeQuotaConfig if it's different from the current status.
func (r *NodeQuotaConfigReconciler) UpdateConfigStatus(ctx context.Context, config danav1alpha1.NodeQuotaConfig, oldConfigStatus danav1alpha1.NodeQuotaConfigStatus, logger logr.Logger) error {
	if !reflect.DeepEqual(oldConfigStatus.ReservedResources, config.Status.ReservedResources) {
		if err := r.Status().Update(ctx, &config); err != nil {
			logger.Error(err, fmt.Sprintf("Error updating the NodeQuotaConfig"))
			return err
		}
	}
	return nil
}

// requestConfigReconcile generates a list of reconcile requests for NodeQuotaConfig objects that need to be reconciled.
// It takes a context and the node object.
// It returns a slice of reconcile requests ([]reconcile.Request).
func (r *NodeQuotaConfigReconciler) requestConfigReconcile(ctx context.Context, node client.Object) []reconcile.Request {
	nodeQuotaConfig := danav1alpha1.NodeQuotaConfigList{}
	err := r.List(ctx, &nodeQuotaConfig)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(nodeQuotaConfig.Items))
	for i, item := range nodeQuotaConfig.Items {
		requests[i] = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		}
	}
	return requests
}
