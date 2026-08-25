package tests

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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/santoshkal/updatr/internal/controller"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	return s
}

func deploymentReferencingSecret(ns, name, secretName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
					Volumes: []corev1.Volume{
						{
							Name: "sec",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: secretName},
							},
						},
					},
				},
			},
		},
	}
}

func deploymentReferencingConfigMap(ns, name, cmName string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Image: "nginx",
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: cmName}}},
							},
						},
					},
				},
			},
		},
	}
}

func statefulSetReferencingSecret(ns, name, secretName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			ServiceName: name,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Image: "nginx",
							Env: []corev1.EnvVar{
								{
									Name: "TOK",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretName}, Key: "k"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func statefulSetReferencingConfigMap(ns, name, cmName string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			ServiceName: name,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
					Volumes: []corev1.Volume{
						{
							Name: "cm",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: cmName}},
							},
						},
					},
				},
			},
		},
	}
}

func nonReferencingDeployment(ns, name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			},
		},
	}
}

func assertDeploymentPatched(t *testing.T, c client.Client, ns, name string, shouldBePatched bool) {
	t.Helper()
	var dep appsv1.Deployment
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &dep))
	_, ok := dep.Spec.Template.Annotations[controller.RestartAnnotation]
	if shouldBePatched {
		assert.True(t, ok, "deployment %s/%s should have been patched with %q", ns, name, controller.RestartAnnotation)
		if ok {
			parsed, err := time.Parse(time.RFC3339Nano, dep.Spec.Template.Annotations[controller.RestartAnnotation])
			assert.NoError(t, err)
			assert.WithinDuration(t, time.Now(), parsed, 5*time.Second)
		}
	} else {
		assert.False(t, ok, "deployment %s/%s should NOT have been patched", ns, name)
	}
}

func assertStatefulSetPatched(t *testing.T, c client.Client, ns, name string, shouldBePatched bool) {
	t.Helper()
	var sts appsv1.StatefulSet
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &sts))
	_, ok := sts.Spec.Template.Annotations[controller.RestartAnnotation]
	if shouldBePatched {
		assert.True(t, ok, "statefulset %s/%s should have been patched", ns, name)
		if ok {
			parsed, err := time.Parse(time.RFC3339Nano, sts.Spec.Template.Annotations[controller.RestartAnnotation])
			assert.NoError(t, err)
			assert.WithinDuration(t, time.Now(), parsed, 5*time.Second)
		}
	} else {
		assert.False(t, ok, "statefulset %s/%s should NOT have been patched", ns, name)
	}
}

// ---------------------------------------------------------------------------
// SecretReconciler
// ---------------------------------------------------------------------------

func TestSecretReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		objects []client.Object
		interceptor *interceptor.Funcs
		req     ctrl.Request
		wantErr bool
		errContains string
		validate func(t *testing.T, c client.Client)
	}{
		{
			name: "NotFound secret returns no error",
			objects: []client.Object{},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "missing"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
			},
		},
		{
			name: "no workloads returns no error and no patches",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
			},
		},
		{
			name: "non-referencing deployment not patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				nonReferencingDeployment("default", "dep-a"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-a", false)
			},
		},
		{
			name: "referencing deployment is patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				deploymentReferencingSecret("default", "dep-ref", "my-secret"),
				nonReferencingDeployment("default", "dep-nope"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-ref", true)
				assertDeploymentPatched(t, c, "default", "dep-nope", false)
			},
		},
		{
			name: "referencing statefulset is patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				statefulSetReferencingSecret("default", "sts-ref", "my-secret"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertStatefulSetPatched(t, c, "default", "sts-ref", true)
			},
		},
		{
			name: "both deployment and statefulset referencing same secret are patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				deploymentReferencingSecret("default", "dep-ref", "my-secret"),
				statefulSetReferencingSecret("default", "sts-ref", "my-secret"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-ref", true)
				assertStatefulSetPatched(t, c, "default", "sts-ref", true)
			},
		},
		{
			name: "namespace isolation - other namespace not patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				deploymentReferencingSecret("default", "dep-default", "my-secret"),
				deploymentReferencingSecret("other", "dep-other", "my-secret"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-default", true)
				assertDeploymentPatched(t, c, "other", "dep-other", false)
			},
		},
		{
			name: "multiple deployments only matching ones patched",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"}},
				deploymentReferencingSecret("default", "dep-target", "target"),
				deploymentReferencingSecret("default", "dep-other", "other-secret"),
				nonReferencingDeployment("default", "dep-none"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "target"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-target", true)
				assertDeploymentPatched(t, c, "default", "dep-other", false)
				assertDeploymentPatched(t, c, "default", "dep-none", false)
			},
		},
		{
			name: "List deployments error returns wrapped error",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
			},
			interceptor: &interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					// intercept only DeploymentList
					if _, ok := list.(*appsv1.DeploymentList); ok {
						return errors.New("injected list error")
					}
					return c.List(ctx, list, opts...)
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			wantErr: true,
			errContains: "listing deployments",
		},
		{
			name: "List statefulsets error returns wrapped error",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
			},
			interceptor: &interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*appsv1.StatefulSetList); ok {
						return errors.New("injected list error")
					}
					return c.List(ctx, list, opts...)
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			wantErr: true,
			errContains: "listing statefulsets",
		},
		{
			name: "Patch deployment error returns wrapped error",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				deploymentReferencingSecret("default", "dep-ref", "my-secret"),
			},
			interceptor: &interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return errors.New("injected patch error")
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			wantErr: true,
			errContains: "patching deployment",
		},
		{
			name: "Patch statefulset error returns wrapped error",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
				statefulSetReferencingSecret("default", "sts-ref", "my-secret"),
			},
			interceptor: &interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return errors.New("injected patch error")
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			wantErr: true,
			errContains: "patching statefulset",
		},
		{
			name: "returns Result with no requeue on success",
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(tt.objects...)
			if tt.interceptor != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptor)
			}
			fakeClient := builder.Build()

			r := &controller.SecretReconciler{
				Client: fakeClient,
				Scheme: testScheme(),
			}

			result, err := r.Reconcile(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)

			if tt.validate != nil {
				tt.validate(t, fakeClient)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ConfigMapReconciler
// ---------------------------------------------------------------------------

func TestConfigMapReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		objects     []client.Object
		interceptor *interceptor.Funcs
		req         ctrl.Request
		wantErr     bool
		errContains string
		validate    func(t *testing.T, c client.Client)
	}{
		{
			name: "NotFound configmap returns no error",
			objects: []client.Object{},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "missing"}},
		},
		{
			name: "no workloads",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
		},
		{
			name: "non-referencing deployment not patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				nonReferencingDeployment("default", "dep-a"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-a", false)
			},
		},
		{
			name: "referencing deployment is patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				deploymentReferencingConfigMap("default", "dep-ref", "my-cm"),
				nonReferencingDeployment("default", "dep-nope"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-ref", true)
				assertDeploymentPatched(t, c, "default", "dep-nope", false)
			},
		},
		{
			name: "referencing statefulset is patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				statefulSetReferencingConfigMap("default", "sts-ref", "my-cm"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertStatefulSetPatched(t, c, "default", "sts-ref", true)
			},
		},
		{
			name: "both deployment and statefulset referencing same cm are patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				deploymentReferencingConfigMap("default", "dep-ref", "my-cm"),
				statefulSetReferencingConfigMap("default", "sts-ref", "my-cm"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-ref", true)
				assertStatefulSetPatched(t, c, "default", "sts-ref", true)
			},
		},
		{
			name: "namespace isolation - other namespace not patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				deploymentReferencingConfigMap("default", "dep-default", "my-cm"),
				deploymentReferencingConfigMap("other", "dep-other", "my-cm"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-default", true)
				assertDeploymentPatched(t, c, "other", "dep-other", false)
			},
		},
		{
			name: "multiple deployments only matching ones patched",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"}},
				deploymentReferencingConfigMap("default", "dep-target", "target"),
				deploymentReferencingConfigMap("default", "dep-other", "other-cm"),
				nonReferencingDeployment("default", "dep-none"),
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "target"}},
			validate: func(t *testing.T, c client.Client) {
				t.Helper()
				assertDeploymentPatched(t, c, "default", "dep-target", true)
				assertDeploymentPatched(t, c, "default", "dep-other", false)
				assertDeploymentPatched(t, c, "default", "dep-none", false)
			},
		},
		{
			name: "List deployments error returns wrapped error",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
			},
			interceptor: &interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*appsv1.DeploymentList); ok {
						return errors.New("injected list error")
					}
					return c.List(ctx, list, opts...)
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			wantErr: true,
			errContains: "listing deployments",
		},
		{
			name: "List statefulsets error returns wrapped error",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
			},
			interceptor: &interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*appsv1.StatefulSetList); ok {
						return errors.New("injected list error")
					}
					return c.List(ctx, list, opts...)
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			wantErr: true,
			errContains: "listing statefulsets",
		},
		{
			name: "Patch deployment error returns wrapped error",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				deploymentReferencingConfigMap("default", "dep-ref", "my-cm"),
			},
			interceptor: &interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return errors.New("injected patch error")
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			wantErr: true,
			errContains: "patching deployment",
		},
		{
			name: "Patch statefulset error returns wrapped error",
			objects: []client.Object{
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}},
				statefulSetReferencingConfigMap("default", "sts-ref", "my-cm"),
			},
			interceptor: &interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					return errors.New("injected patch error")
				},
			},
			req: ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}},
			wantErr: true,
			errContains: "patching statefulset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(tt.objects...)
			if tt.interceptor != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptor)
			}
			fakeClient := builder.Build()

			r := &controller.ConfigMapReconciler{
				Client: fakeClient,
				Scheme: testScheme(),
			}

			result, err := r.Reconcile(context.Background(), tt.req)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, ctrl.Result{}, result)

			if tt.validate != nil {
				tt.validate(t, fakeClient)
			}
		})
	}
}

func TestReconcilers_IndependentRunnable(t *testing.T) {
	t.Parallel()

	// Verify two reconcilers operating on same namespace do not interfere cross-type:
	// A Secret reconcile must not restart ConfigMap consumers and vice versa.
	scheme := testScheme()

	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}}
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}}
	depSecret := deploymentReferencingSecret("default", "dep-secret", "my-secret")
	depCM := deploymentReferencingConfigMap("default", "dep-cm", "my-cm")

	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, cm, depSecret, depCM)
	c := builder.Build()

	// Reconcile Secret -> only secret consumer patched.
	secretR := &controller.SecretReconciler{Client: c, Scheme: scheme}
	_, err := secretR.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}})
	require.NoError(t, err)
	assertDeploymentPatched(t, c, "default", "dep-secret", true)
	assertDeploymentPatched(t, c, "default", "dep-cm", false)

	// Reset by rebuilding client without annotation (simulate fresh).
	c2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, cm, depSecret, depCM).Build()
	cmR := &controller.ConfigMapReconciler{Client: c2, Scheme: scheme}
	_, err = cmR.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}})
	require.NoError(t, err)
	assertDeploymentPatched(t, c2, "default", "dep-cm", true)
	assertDeploymentPatched(t, c2, "default", "dep-secret", false)
}

func TestController_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("Secret Get generic error propagates (not NotFound) and is wrapped", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "default"}}
		dep := deploymentReferencingSecret("default", "dep-ref", "my-secret")
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, dep).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cw client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// intercept Secret Get only
				if _, ok := obj.(*corev1.Secret); ok && key.Name == "my-secret" {
					return errors.New("etcd timeout")
				}
				return cw.Get(ctx, key, obj, opts...)
			},
		}).Build()
		r := &controller.SecretReconciler{Client: c, Scheme: scheme}
		// Current implementation: `return ctrl.Result{}, client.IgnoreNotFound(err)` -> IgnoreNotFound returns non-nil for non-NotFound error
		// so Reconcile returns that error (no wrapping). We assert error is returned.
		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-secret"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "etcd timeout")
	})

	t.Run("ConfigMap Get generic error propagates", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"}}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cw client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.ConfigMap); ok {
					return errors.New("api server unavailable")
				}
				return cw.Get(ctx, key, obj, opts...)
			},
		}).Build()
		r := &controller.ConfigMapReconciler{Client: c, Scheme: scheme}
		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "my-cm"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "api server unavailable")
	})

	t.Run("workload referencing same secret via volume+env patches only once per Reconcile", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "dup-secret", Namespace: "default"}}
		// Deployment references same secret twice: volume and env
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-dup", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dup"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "dup-secret"}}}},
						Containers: []corev1.Container{
							{
								Name: "app", Image: "nginx",
								Env: []corev1.EnvVar{{Name: "X", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "dup-secret"}, Key: "k"}}}},
								EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "dup-secret"}}}},
							},
						},
					},
				},
			},
		}
		patchCount := 0
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, dep).WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cw client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patchCount++
				return cw.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
		r := &controller.SecretReconciler{Client: c, Scheme: scheme}
		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "dup-secret"}})
		require.NoError(t, err)
		assert.Equal(t, 1, patchCount, "deployment with multiple refs to same secret should be patched exactly once")
		assertDeploymentPatched(t, c, "default", "dep-dup", true)
	})

	t.Run("workload referencing via initContainer and ephemeralContainer is detected", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "peculiar-secret", Namespace: "default"}}
		depInit := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-init", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "init"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Name: "init", Image: "busybox", Env: []corev1.EnvVar{{Name: "X", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "peculiar-secret"}, Key: "k"}}}}}},
						Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
						EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "peculiar-secret"}}}}}}},
					},
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, depInit).Build()
		r := &controller.SecretReconciler{Client: c, Scheme: scheme}
		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "peculiar-secret"}})
		require.NoError(t, err)
		assertDeploymentPatched(t, c, "default", "dep-init", true)
	})

	t.Run("empty request NamespacedName returns NotFound without error", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &controller.SecretReconciler{Client: c, Scheme: scheme}
		result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{}})
		require.NoError(t, err)
		assert.Equal(t, ctrl.Result{}, result)
	})

	t.Run("projected volume with both secret and configmap same name only secret reconciler patches", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"}}
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-shared", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "shared"}},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "proj",
								VolumeSource: corev1.VolumeSource{
									Projected: &corev1.ProjectedVolumeSource{
										Sources: []corev1.VolumeProjection{
											{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "shared"}}},
										},
									},
								},
							},
						},
						Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
					},
				},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret, dep).Build()
		// Secret reconcile should patch
		secretR := &controller.SecretReconciler{Client: c, Scheme: scheme}
		_, err := secretR.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "shared"}})
		require.NoError(t, err)
		assertDeploymentPatched(t, c, "default", "dep-shared", true)

		// Reset and test ConfigMap reconciler does NOT patch same dep when only Secret projection exists
		c2 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"}}, dep).Build()
		cmR := &controller.ConfigMapReconciler{Client: c2, Scheme: scheme}
		_, err = cmR.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "shared"}})
		require.NoError(t, err)
		assertDeploymentPatched(t, c2, "default", "dep-shared", false)
	})

	t.Run("List with many workloads verifies only matching subset patched (count check)", func(t *testing.T) {
		t.Parallel()
		scheme := testScheme()
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "target", Namespace: "default"}}
		objs := []client.Object{secret}
		for i := 0; i < 10; i++ {
			if i%3 == 0 {
				objs = append(objs, deploymentReferencingSecret("default", "dep-match-"+string(rune('a'+i)), "target"))
			} else {
				objs = append(objs, nonReferencingDeployment("default", "dep-no-match-"+string(rune('a'+i))))
			}
		}
		patchCount := 0
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, cw client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
				patchCount++
				return cw.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
		r := &controller.SecretReconciler{Client: c, Scheme: scheme}
		_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "target"}})
		require.NoError(t, err)
		// i = 0,3,6,9 => 4 matches
		assert.Equal(t, 4, patchCount)
	})
}
