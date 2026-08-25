package tests

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pred "github.com/santoshkal/updatr/internal/predicate"
)

func TestResourceVersionChanged(t *testing.T) {
	t.Parallel()

	p := pred.ResourceVersionChanged()
	require.NotNil(t, p)

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "CreateFunc always returns false",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Create(event.CreateEvent{Object: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x", ResourceVersion: "1"}}})
				assert.False(t, got)
			},
		},
		{
			name: "CreateFunc returns false even with nil object",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Create(event.CreateEvent{})
				assert.False(t, got)
			},
		},
		{
			name: "DeleteFunc always returns false",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Delete(event.DeleteEvent{Object: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "x", ResourceVersion: "1"}}})
				assert.False(t, got)
			},
		},
		{
			name: "DeleteFunc returns false even with nil object",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Delete(event.DeleteEvent{})
				assert.False(t, got)
			},
		},
		{
			name: "GenericFunc always returns false",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Generic(event.GenericEvent{Object: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x", ResourceVersion: "1"}}})
				assert.False(t, got)
			},
		},
		{
			name: "GenericFunc returns false with nil object",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Generic(event.GenericEvent{})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns false when ObjectOld is nil",
			test: func(t *testing.T) {
				t.Helper()
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "2"}}
				got := p.Update(event.UpdateEvent{ObjectOld: nil, ObjectNew: newObj})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns false when ObjectNew is nil",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: nil})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns false when both nil",
			test: func(t *testing.T) {
				t.Helper()
				got := p.Update(event.UpdateEvent{ObjectOld: nil, ObjectNew: nil})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns false when resourceVersion unchanged",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1001"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1001"}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns false when both empty RV",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: ""}}
				newObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: ""}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
				assert.False(t, got)
			},
		},
		{
			name: "UpdateFunc returns true when resourceVersion changed",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1001"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1002"}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
				assert.True(t, got)
			},
		},
		{
			name: "UpdateFunc returns true when RV changes from empty to non-empty",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: ""}}
				newObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
				assert.True(t, got)
			},
		},
		{
			name: "UpdateFunc handles Secret vs ConfigMap types transparently",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "500"}}
				newObj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "501"}}
				got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
				assert.True(t, got)
			},
		},
		{
			name: "UpdateFunc detects numeric RV increment vs same",
			test: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "9999"}}
				newObjSame := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "9999"}}
				assert.False(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObjSame}))

				newObjDiff := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "10000"}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObjDiff}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}

func TestResourceVersionChanged_TableDrivenUpdates(t *testing.T) {
	t.Parallel()

	p := pred.ResourceVersionChanged()

	type rvCase struct {
		name   string
		oldRV  string
		newRV  string
		expect bool
	}

	cases := []rvCase{
		{name: "same RV", oldRV: "1", newRV: "1", expect: false},
		{name: "incremented RV", oldRV: "1", newRV: "2", expect: true},
		{name: "empty to empty", oldRV: "", newRV: "", expect: false},
		{name: "empty to value", oldRV: "", newRV: "42", expect: true},
		{name: "value to empty", oldRV: "42", newRV: "", expect: true},
		{name: "large numbers same", oldRV: "123456789", newRV: "123456789", expect: false},
		{name: "large numbers different", oldRV: "123456789", newRV: "123456790", expect: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: tc.oldRV}}
			newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: tc.newRV}}
			got := p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj})
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestResourceVersionChanged_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	p := pred.ResourceVersionChanged()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "whitespace difference is considered changed (opaque string)",
			run: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: " 1"}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}))
				// identical whitespace remains false
				sameSpace := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: " 1"}}
				assert.False(t, p.Update(event.UpdateEvent{ObjectOld: newObj, ObjectNew: sameSpace}))
			},
		},
		{
			name: "leading zeros difference is change",
			run: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "001"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}))
			},
		},
		{
			name: "same object pointer with same RV returns false",
			run: func(t *testing.T) {
				t.Helper()
				obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "42"}}
				assert.False(t, p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj}))
			},
		},
		{
			name: "same pointer after RV mutated returns true",
			run: func(t *testing.T) {
				t.Helper()
				obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				oldRV := obj.GetResourceVersion()
				obj.SetResourceVersion("2")
				// Simulate UpdateEvent where old snapshot and new object share mutation history:
				// We explicitly create separate old object preserving old RV.
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: oldRV}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: obj}))
			},
		},
		{
			name: "Create/Delete/Generic remain false even with RV present",
			run: func(t *testing.T) {
				t.Helper()
				secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "x", ResourceVersion: "999"}}
				assert.False(t, p.Create(event.CreateEvent{Object: secret}))
				assert.False(t, p.Delete(event.DeleteEvent{Object: secret}))
				assert.False(t, p.Generic(event.GenericEvent{Object: secret}))
			},
		},
		{
			name: "predicate instances are independent and stateless",
			run: func(t *testing.T) {
				t.Helper()
				p2 := pred.ResourceVersionChanged()
				require.NotNil(t, p2)
				// different instances behave identically
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "2"}}
				assert.Equal(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}), p2.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}))
			},
		},
		{
			name: "RV with newline is treated as different string",
			run: func(t *testing.T) {
				t.Helper()
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1\n"}}
				newObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "1"}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObj}))
			},
		},
		{
			name: "extremely long RV same vs different",
			run: func(t *testing.T) {
				t.Helper()
				long := string(make([]byte, 1000))
				for i := range 1000 {
					long = long[:i] + "a"
				}
				// use 256 char RVs
				rvLong1 := "1234567890" + string(make([]byte, 256))
				rvLong2 := rvLong1 + "x"
				oldObj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: rvLong1}}
				newObjSame := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: rvLong1}}
				assert.False(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObjSame}))
				newObjDiff := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{ResourceVersion: rvLong2}}
				assert.True(t, p.Update(event.UpdateEvent{ObjectOld: oldObj, ObjectNew: newObjDiff}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
