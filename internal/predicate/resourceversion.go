// Package predicate provides event filters for the updatr controller.
package predicate

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ResourceVersionChanged returns a predicate that only allows update events
// where the object's resourceVersion has changed.
//
// This prevents unwanted reconciles on resyncs, status updates, or
// periodic cache resyncs where the resourceVersion stays identical. The
// controller-runtime cache generates Update events even when nothing changed;
// filtering on resourceVersion ensures we only react to actual writes.
func ResourceVersionChanged() predicate.Predicate {
	// predicate.Funcs builds a predicate from per-event callbacks;
	// we return it so callers can pass it to WithEventFilter.
	return predicate.Funcs{
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == nil || e.ObjectNew == nil {
				return false
			}

			// GetResourceVersion() returns the opaque resourceVersion string
			// that the API server bumps on every persisted change.
			oldRV := e.ObjectOld.GetResourceVersion()
			// GetResourceVersion() on the new object to compare against old.
			newRV := e.ObjectNew.GetResourceVersion()

			return oldRV != newRV
		},
	}
}
