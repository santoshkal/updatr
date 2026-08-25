package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pred "github.com/santoshkal/updatr/internal/predicate"
	"github.com/santoshkal/updatr/internal/resources"
)

// ConfigMapReconciler watches ConfigMaps and restarts workloads that consume them.
//
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;patch
type ConfigMapReconciler struct {
	// Client is the controller-runtime client for API operations.
	client.Client
	// Scheme is the runtime scheme.
	Scheme *runtime.Scheme
}

// Reconcile reacts to ConfigMap updates where resourceVersion changed.
// It lists Deployments and StatefulSets in the same namespace and triggers
// a rollout for each that references the ConfigMap.
func (r *ConfigMapReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	// log.FromContext() retrieves the request-scoped logger.
	logger := log.FromContext(ctx)
	// logger.Info() logs the start of ConfigMap reconciliation.
	logger.Info("reconciling ConfigMap", "configmap", req.NamespacedName)

	var cm corev1.ConfigMap
	// r.Get() fetches the ConfigMap by NamespacedName.
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		// client.IgnoreNotFound() suppresses NotFound errors for deleted objects.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// r.restartDeploymentsForConfigMap() patches Deployments consuming this ConfigMap.
	if err := r.restartDeploymentsForConfigMap(ctx, &cm); err != nil {
		// fmt.Errorf() wraps the deployment restart error with identity.
		return ctrl.Result{}, fmt.Errorf("restarting deployments for configmap %q: %w", req.NamespacedName, err)
	}

	// r.restartStatefulSetsForConfigMap() patches StatefulSets consuming this ConfigMap.
	if err := r.restartStatefulSetsForConfigMap(ctx, &cm); err != nil {
		// fmt.Errorf() wraps the statefulset error.
		return ctrl.Result{}, fmt.Errorf("restarting statefulsets for configmap %q: %w", req.NamespacedName, err)
	}

	// ctrl.Result{} indicates successful reconciliation without requeue.
	return ctrl.Result{}, nil
}

func (r *ConfigMapReconciler) restartDeploymentsForConfigMap(
	ctx context.Context,
	cm *corev1.ConfigMap,
) error {
	var list appsv1.DeploymentList
	// r.List() lists Deployments in the ConfigMap's namespace.
	if err := r.List(ctx, &list, client.InNamespace(cm.Namespace)); err != nil {
		// fmt.Errorf() annotates the list failure.
		return fmt.Errorf("listing deployments in namespace %q: %w", cm.Namespace, err)
	}

	// range iterates over Deployments to check ConfigMap references.
	for i := range list.Items {
		dep := &list.Items[i]

		// resources.PodSpecReferencesConfigMap() checks volumes/env for ConfigMap usage.
		if !resources.PodSpecReferencesConfigMap(&dep.Spec.Template.Spec, cm.Name) {
			continue
		}

		// triggerDeploymentRollout() patches the Deployment to restart pods.
		if err := triggerDeploymentRollout(ctx, r.Client, dep); err != nil {
			// fmt.Errorf() wraps the patch error with %q.
			return fmt.Errorf("patching deployment %q/%q: %w", dep.Namespace, dep.Name, err)
		}
	}

	return nil
}

func (r *ConfigMapReconciler) restartStatefulSetsForConfigMap(
	ctx context.Context,
	cm *corev1.ConfigMap,
) error {
	var list appsv1.StatefulSetList
	// r.List() lists StatefulSets in the ConfigMap's namespace.
	if err := r.List(ctx, &list, client.InNamespace(cm.Namespace)); err != nil {
		// fmt.Errorf() annotates the list error.
		return fmt.Errorf("listing statefulsets in namespace %q: %w", cm.Namespace, err)
	}

	// range iterates over StatefulSets to filter by ConfigMap usage.
	for i := range list.Items {
		sts := &list.Items[i]

		// resources.PodSpecReferencesConfigMap() checks PodSpec for ConfigMap reference.
		if !resources.PodSpecReferencesConfigMap(&sts.Spec.Template.Spec, cm.Name) {
			continue
		}

		// triggerStatefulSetRollout() patches the StatefulSet to trigger a rollout.
		if err := triggerStatefulSetRollout(ctx, r.Client, sts); err != nil {
			// fmt.Errorf() wraps the patch error with %q.
			return fmt.Errorf("patching statefulset %q/%q: %w", sts.Namespace, sts.Name, err)
		}
	}

	return nil
}

// SetupWithManager registers the ConfigMapReconciler with the manager.
// The ResourceVersionChanged predicate ensures we only reconcile on actual
// data changes (resourceVersion bumps), not on periodic resyncs.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// pred.ResourceVersionChanged() builds the filter that compares old vs new RV.
	rvPredicate := pred.ResourceVersionChanged()
	// ctrl.NewControllerManagedBy() creates a controller builder.
	// For() sets ConfigMap as the primary resource.
	// WithEventFilter() installs the RV predicate.
	// Complete() finalizes registration.
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(rvPredicate).
		Complete(r)
}
