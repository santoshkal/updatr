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

// SecretReconciler watches Secrets and restarts workloads that consume them.
//
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;patch
type SecretReconciler struct {
	// Client is the controller-runtime client for API operations.
	client.Client
	// Scheme is the runtime scheme for type conversion.
	Scheme *runtime.Scheme
}

// Reconcile reacts to Secret updates where resourceVersion changed.
// It lists Deployments and StatefulSets in the same namespace and triggers
// a rollout for each that references the Secret via volumes or env.
func (r *SecretReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	// log.FromContext() fetches the request-scoped logger.
	logger := log.FromContext(ctx)
	// logger.Info() logs the start of reconciliation at info level.
	logger.Info("reconciling Secret", "secret", req.NamespacedName)

	var secret corev1.Secret
	// r.Get() fetches the Secret from the API server / cache by NamespacedName.
	if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
		// client.IgnoreNotFound() returns nil if the error is NotFound, suppressing deleted objects.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// r.restartDeploymentsForSecret() finds and patches Deployments referencing this Secret.
	if err := r.restartDeploymentsForSecret(ctx, &secret); err != nil {
		// fmt.Errorf() wraps the error with context for the requeue.
		return ctrl.Result{}, fmt.Errorf("restarting deployments for secret %q: %w", req.NamespacedName, err)
	}

	// r.restartStatefulSetsForSecret() finds and patches StatefulSets referencing this Secret.
	if err := r.restartStatefulSetsForSecret(ctx, &secret); err != nil {
		// fmt.Errorf() wraps the StatefulSet rollout error.
		return ctrl.Result{}, fmt.Errorf("restarting statefulsets for secret %q: %w", req.NamespacedName, err)
	}

	// ctrl.Result{} signals successful reconciliation with no requeue.
	return ctrl.Result{}, nil
}

func (r *SecretReconciler) restartDeploymentsForSecret(
	ctx context.Context,
	secret *corev1.Secret,
) error {
	var list appsv1.DeploymentList
	// r.List() lists Deployments in the Secret's namespace.
	if err := r.List(ctx, &list, client.InNamespace(secret.Namespace)); err != nil {
		// fmt.Errorf() annotates the list error with namespace.
		return fmt.Errorf("listing deployments in namespace %q: %w", secret.Namespace, err)
	}

	// range iterates over Deployments to check Secret references.
	for i := range list.Items {
		dep := &list.Items[i]

		// resources.PodSpecReferencesSecret() checks PodSpec for Secret usage (volumes/env).
		if !resources.PodSpecReferencesSecret(&dep.Spec.Template.Spec, secret.Name) {
			continue
		}

		// triggerDeploymentRollout() patches the Deployment to trigger a rolling restart.
		if err := triggerDeploymentRollout(ctx, r.Client, dep); err != nil {
			// fmt.Errorf() wraps the patch error with %q for visible boundaries.
			return fmt.Errorf("patching deployment %q/%q: %w", dep.Namespace, dep.Name, err)
		}
	}

	return nil
}

func (r *SecretReconciler) restartStatefulSetsForSecret(
	ctx context.Context,
	secret *corev1.Secret,
) error {
	var list appsv1.StatefulSetList
	// r.List() lists StatefulSets in the Secret's namespace.
	if err := r.List(ctx, &list, client.InNamespace(secret.Namespace)); err != nil {
		// fmt.Errorf() annotates the list error.
		return fmt.Errorf("listing statefulsets in namespace %q: %w", secret.Namespace, err)
	}

	// range iterates over StatefulSets to filter by Secret reference.
	for i := range list.Items {
		sts := &list.Items[i]

		// resources.PodSpecReferencesSecret() checks if the PodSpec uses this Secret.
		if !resources.PodSpecReferencesSecret(&sts.Spec.Template.Spec, secret.Name) {
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

// SetupWithManager registers the SecretReconciler with the manager.
// It watches Secrets with a predicate that only allows updates where
// resourceVersion changed, preventing spurious reconciles on resyncs.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// pred.ResourceVersionChanged() returns the RV-change filter predicate.
	rvPredicate := pred.ResourceVersionChanged()
	// ctrl.NewControllerManagedBy() starts building a controller for this reconciler.
	// For() declares the primary watched type (Secret).
	// WithEventFilter() attaches the ResourceVersion predicate.
	// Complete() registers the controller with the manager.
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(rvPredicate).
		Complete(r)
}
