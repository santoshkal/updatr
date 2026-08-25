package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	return s
}

func newDeployment(ns, name string, annotations map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			},
		},
	}
}

func newStatefulSet(ns, name string, annotations map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			ServiceName: name,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			},
		},
	}
}

func TestTriggerDeploymentRollout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		deployment *appsv1.Deployment
		prepareClient func(*appsv1.Deployment) client.Client
		wantErr     bool
		validate    func(t *testing.T, dep *appsv1.Deployment, c client.Client)
	}{
		{
			name:        "nil annotations map gets initialized and annotation set",
			deployment: newDeployment("default", "dep-nil", nil),
			prepareClient: func(dep *appsv1.Deployment) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
			},
			validate: func(t *testing.T, dep *appsv1.Deployment, c client.Client) {
				t.Helper()
				var fetched appsv1.Deployment
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dep), &fetched))
				ann := fetched.Spec.Template.Annotations
				require.NotNil(t, ann)
				val, ok := ann[RestartAnnotation]
				require.True(t, ok, "annotation %q should exist", RestartAnnotation)
				parsed, err := time.Parse(time.RFC3339Nano, val)
				require.NoError(t, err, "annotation should be RFC3339Nano")
				assert.WithinDuration(t, time.Now(), parsed, 5*time.Second)
			},
		},
		{
			name:        "existing annotations are preserved",
			deployment: newDeployment("default", "dep-preserve", map[string]string{"existing": "keep-me", "foo": "bar"}),
			prepareClient: func(dep *appsv1.Deployment) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
			},
			validate: func(t *testing.T, dep *appsv1.Deployment, c client.Client) {
				t.Helper()
				var fetched appsv1.Deployment
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dep), &fetched))
				assert.Equal(t, "keep-me", fetched.Spec.Template.Annotations["existing"])
				assert.Equal(t, "bar", fetched.Spec.Template.Annotations["foo"])
				_, ok := fetched.Spec.Template.Annotations[RestartAnnotation]
				assert.True(t, ok)
			},
		},
		{
			name:        "existing RestartAnnotation is overwritten with new timestamp",
			deployment: newDeployment("default", "dep-overwrite", map[string]string{RestartAnnotation: "2020-01-01T00:00:00Z"}),
			prepareClient: func(dep *appsv1.Deployment) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
			},
			validate: func(t *testing.T, dep *appsv1.Deployment, c client.Client) {
				t.Helper()
				var fetched appsv1.Deployment
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dep), &fetched))
				val := fetched.Spec.Template.Annotations[RestartAnnotation]
				assert.NotEqual(t, "2020-01-01T00:00:00Z", val)
				_, err := time.Parse(time.RFC3339Nano, val)
				assert.NoError(t, err)
			},
		},
		{
			name:        "patch error is wrapped and returned",
			deployment: newDeployment("default", "dep-err", nil),
			prepareClient: func(dep *appsv1.Deployment) client.Client {
				return fake.NewClientBuilder().
					WithScheme(newScheme()).
					WithObjects(dep).
					WithInterceptorFuncs(interceptor.Funcs{
						Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
							return errors.New("injected patch error")
						},
					}).
					Build()
			},
			wantErr: true,
		},
		{
			name:        "empty annotations map (non-nil) also gets annotation",
			deployment: newDeployment("default", "dep-empty", map[string]string{}),
			prepareClient: func(dep *appsv1.Deployment) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
			},
			validate: func(t *testing.T, dep *appsv1.Deployment, c client.Client) {
				t.Helper()
				var fetched appsv1.Deployment
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dep), &fetched))
				_, ok := fetched.Spec.Template.Annotations[RestartAnnotation]
				assert.True(t, ok)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.prepareClient(tt.deployment)
			err := triggerDeploymentRollout(context.Background(), c, tt.deployment)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "patch deployment")
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, tt.deployment, c)
			}
		})
	}
}

func TestTriggerStatefulSetRollout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sts         *appsv1.StatefulSet
		prepareClient func(*appsv1.StatefulSet) client.Client
		wantErr     bool
		validate    func(t *testing.T, sts *appsv1.StatefulSet, c client.Client)
	}{
		{
			name: "nil annotations map gets initialized and annotation set",
			sts:  newStatefulSet("default", "sts-nil", nil),
			prepareClient: func(s *appsv1.StatefulSet) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(s).Build()
			},
			validate: func(t *testing.T, s *appsv1.StatefulSet, c client.Client) {
				t.Helper()
				var fetched appsv1.StatefulSet
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(s), &fetched))
				val, ok := fetched.Spec.Template.Annotations[RestartAnnotation]
				require.True(t, ok)
				parsed, err := time.Parse(time.RFC3339Nano, val)
				require.NoError(t, err)
				assert.WithinDuration(t, time.Now(), parsed, 5*time.Second)
			},
		},
		{
			name: "existing annotations are preserved",
			sts:  newStatefulSet("default", "sts-preserve", map[string]string{"keep": "yes"}),
			prepareClient: func(s *appsv1.StatefulSet) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(s).Build()
			},
			validate: func(t *testing.T, s *appsv1.StatefulSet, c client.Client) {
				t.Helper()
				var fetched appsv1.StatefulSet
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(s), &fetched))
				assert.Equal(t, "yes", fetched.Spec.Template.Annotations["keep"])
				_, ok := fetched.Spec.Template.Annotations[RestartAnnotation]
				assert.True(t, ok)
			},
		},
		{
			name: "existing RestartAnnotation is overwritten",
			sts:  newStatefulSet("default", "sts-overwrite", map[string]string{RestartAnnotation: "old"}),
			prepareClient: func(s *appsv1.StatefulSet) client.Client {
				return fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(s).Build()
			},
			validate: func(t *testing.T, s *appsv1.StatefulSet, c client.Client) {
				t.Helper()
				var fetched appsv1.StatefulSet
				require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(s), &fetched))
				assert.NotEqual(t, "old", fetched.Spec.Template.Annotations[RestartAnnotation])
			},
		},
		{
			name: "patch error is wrapped and returned",
			sts:  newStatefulSet("default", "sts-err", nil),
			prepareClient: func(s *appsv1.StatefulSet) client.Client {
				return fake.NewClientBuilder().
					WithScheme(newScheme()).
					WithObjects(s).
					WithInterceptorFuncs(interceptor.Funcs{
						Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
							return errors.New("injected patch error")
						},
					}).
					Build()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.prepareClient(tt.sts)
			err := triggerStatefulSetRollout(context.Background(), c, tt.sts)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "patch statefulset")
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, tt.sts, c)
			}
		})
	}
}

func TestRestartAnnotationConstant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "updatr.github.com/restartedAt", RestartAnnotation)
}

func TestTriggerDeploymentRollout_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("consecutive patches produce different timestamps (nanosecond uniqueness)", func(t *testing.T) {
		t.Parallel()
		dep := newDeployment("default", "dep-consec", nil)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()

		require.NoError(t, triggerDeploymentRollout(context.Background(), c, dep))
		var first appsv1.Deployment
		require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "dep-consec", Namespace: "default"}, &first))
		firstVal := first.Spec.Template.Annotations[RestartAnnotation]
		require.NotEmpty(t, firstVal)

		// Refetch latest object for second patch (required: DeepCopy inside rollout uses latest)
		var secondDep appsv1.Deployment
		require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "dep-consec", Namespace: "default"}, &secondDep))
		// Ensure clock advances at least 1ns - busy loop or time.Sleep
		time.Sleep(time.Microsecond)
		require.NoError(t, triggerDeploymentRollout(context.Background(), c, &secondDep))
		var second appsv1.Deployment
		require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "dep-consec", Namespace: "default"}, &second))
		secondVal := second.Spec.Template.Annotations[RestartAnnotation]
		require.NotEmpty(t, secondVal)
		// Timestamps must differ; both parse as RFC3339Nano
		parsedFirst, err := time.Parse(time.RFC3339Nano, firstVal)
		require.NoError(t, err)
		parsedSecond, err := time.Parse(time.RFC3339Nano, secondVal)
		require.NoError(t, err)
		assert.True(t, parsedSecond.After(parsedFirst) || !parsedSecond.Equal(parsedFirst), "second timestamp %q should differ from first %q", secondVal, firstVal)
		require.NotEqual(t, firstVal, secondVal)
	})

	t.Run("patch preserves unrelated fields like replicas and labels", func(t *testing.T) {
		t.Parallel()
		replicas := int32(3)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-preserve-fields", Namespace: "default", Labels: map[string]string{"keep": "label"}},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"pre": "existing"}, Labels: map[string]string{"pod": "label"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
		require.NoError(t, triggerDeploymentRollout(context.Background(), c, dep))
		var fetched appsv1.Deployment
		require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "dep-preserve-fields", Namespace: "default"}, &fetched))
		assert.Equal(t, "label", fetched.Labels["keep"])
		assert.Equal(t, int32(3), *fetched.Spec.Replicas)
		assert.Equal(t, "existing", fetched.Spec.Template.Annotations["pre"])
		assert.Equal(t, "label", fetched.Spec.Template.Labels["pod"])
		assert.Contains(t, fetched.Spec.Template.Annotations, RestartAnnotation)
	})

	t.Run("not-found patch error is wrapped with namespaced name", func(t *testing.T) {
		t.Parallel()
		dep := newDeployment("default", "missing-dep", nil)
		// Do NOT add to fake client -> Patch will error (or create not-found behavior)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cw client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return errors.New("not found")
			},
		}).Build()
		err := triggerDeploymentRollout(context.Background(), c, dep)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "patch deployment")
		assert.Contains(t, err.Error(), "missing-dep")
	})

	t.Run("deployment with nil map and empty map both handled without panic", func(t *testing.T) {
		t.Parallel()
		for _, ann := range []map[string]string{nil, {}, make(map[string]string)} {
			dep := newDeployment("default", "dep-nil-edge", ann)
			c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(dep).Build()
			require.NoError(t, triggerDeploymentRollout(context.Background(), c, dep))
			var fetched appsv1.Deployment
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dep), &fetched))
			assert.NotNil(t, fetched.Spec.Template.Annotations)
			assert.NotEmpty(t, fetched.Spec.Template.Annotations[RestartAnnotation])
		}
	})
}

func TestTriggerStatefulSetRollout_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("preserves pod template labels", func(t *testing.T) {
		t.Parallel()
		sts := newStatefulSet("default", "sts-labels", map[string]string{"pre": "val"})
		sts.Spec.Template.Labels = map[string]string{"keep": "yes"}
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sts).Build()
		require.NoError(t, triggerStatefulSetRollout(context.Background(), c, sts))
		var fetched appsv1.StatefulSet
		require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(sts), &fetched))
		assert.Equal(t, "yes", fetched.Spec.Template.Labels["keep"])
		assert.Equal(t, "val", fetched.Spec.Template.Annotations["pre"])
	})

	t.Run("conflict error wrapped", func(t *testing.T) {
		t.Parallel()
		sts := newStatefulSet("default", "sts-conflict", nil)
		c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(sts).WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cw client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				return errors.New("conflict: object has been modified")
			},
		}).Build()
		err := triggerStatefulSetRollout(context.Background(), c, sts)
		require.Error(t, err)
		assert.ErrorContains(t, err, "patch statefulset")
	})
}
