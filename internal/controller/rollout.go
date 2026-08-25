// Package controller implements reconciliation for Secrets and ConfigMaps.
package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// RestartAnnotation is patched onto PodTemplate annotations to trigger a rolling restart.
	// The value is an RFC3339Nano timestamp of the restart request.
	RestartAnnotation = "updatr.github.com/restartedAt"
)

// triggerDeploymentRollout patches the Deployment's PodTemplate to force a rollout.
// It sets/updates RestartAnnotation with the current time, which changes the
// PodTemplate hash and causes the Deployment controller to create new ReplicaSets.
func triggerDeploymentRollout(ctx context.Context, c client.Client, deployment *appsv1.Deployment) error {
	// log.FromContext() retrieves the logger stored in ctx by controller-runtime.
	logger := log.FromContext(ctx)
	// deployment.DeepCopy() creates a deep copy for use as the patch base (original).
	original := deployment.DeepCopy()

	if deployment.Spec.Template.Annotations == nil {
		// make() initializes the annotations map to avoid nil-map assignment panic.
		deployment.Spec.Template.Annotations = make(map[string]string, 1)
	}

	// time.Now() returns the current time for the restart annotation value.
	// time.RFC3339Nano formats with nanosecond precision for uniqueness.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	deployment.Spec.Template.Annotations[RestartAnnotation] = now

	// client.MergeFrom() creates a merge-patch that diffs original vs modified object.
	patch := client.MergeFrom(original)
	// c.Patch() sends the patch to the API server to persist the annotation change.
	if err := c.Patch(ctx, deployment, patch); err != nil {
		// fmt.Errorf() wraps the patch error with %q and %w for visible boundaries and unwrapping.
		return fmt.Errorf(
			"patch deployment %q: %w",
			types.NamespacedName{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			},
			err,
		)
	}

	// logger.Info() logs the rollout trigger at info level with key-value pairs.
	logger.Info(
		"triggered deployment rollout",
		"deployment",
		types.NamespacedName{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
		"annotation",
		RestartAnnotation,
	)

	return nil
}

// triggerStatefulSetRollout patches the StatefulSet's PodTemplate to force a rollout.
// It mirrors triggerDeploymentRollout but for StatefulSet controller semantics.
func triggerStatefulSetRollout(ctx context.Context, c client.Client, statefulSet *appsv1.StatefulSet) error {
	// log.FromContext() retrieves the contextual logger.
	logger := log.FromContext(ctx)
	// statefulSet.DeepCopy() snapshots the original object for merge-patch diff.
	original := statefulSet.DeepCopy()

	if statefulSet.Spec.Template.Annotations == nil {
		// make() initializes the annotations map if nil.
		statefulSet.Spec.Template.Annotations = make(map[string]string, 1)
	}

	// time.Now() captures current time.
	// Format() encodes it as RFC3339Nano.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statefulSet.Spec.Template.Annotations[RestartAnnotation] = now

	// client.MergeFrom() builds a strategic/merge patch from the diff.
	patch := client.MergeFrom(original)
	// c.Patch() applies the patch via the API server.
	if err := c.Patch(ctx, statefulSet, patch); err != nil {
		// fmt.Errorf() wraps the StatefulSet patch error with %q and %w.
		return fmt.Errorf(
			"patch statefulset %q: %w",
			types.NamespacedName{
				Name:      statefulSet.Name,
				Namespace: statefulSet.Namespace,
			},
			err,
		)
	}

	// logger.Info() records the StatefulSet rollout event.
	logger.Info(
		"triggered statefulset rollout",
		"statefulset",
		types.NamespacedName{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
		},
		"annotation",
		RestartAnnotation,
	)

	return nil
}
