package tests

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	"github.com/santoshkal/updatr/internal/resources"
)

func TestPodSpecReferencesSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		podSpec    *corev1.PodSpec
		secretName string
		want       bool
	}{
		{
			name:       "nil PodSpec",
			podSpec:    nil,
			secretName: "my-secret",
			want:       false,
		},
		{
			name:       "empty PodSpec",
			podSpec:    &corev1.PodSpec{},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "volume secret direct match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "sec-vol",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "volume secret name mismatch",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "sec-vol",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "other-secret"},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "projected volume secret match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "projected volume secret mismatch",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "projected volume mixed - second source matches",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "cm-a"}}},
									{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "projected volume with nil Projected",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "empty", VolumeSource: corev1.VolumeSource{}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "downwardAPI projected source does not match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{DownwardAPI: &corev1.DownwardAPIProjection{}},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container env secretKeyRef match",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "TOKEN",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "token"},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "container env secretKeyRef mismatch",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "TOKEN",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "other-secret"}, Key: "token"},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container env with nil ValueFrom",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Env: []corev1.EnvVar{{Name: "FOO", Value: "bar"}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container env ValueFrom with ConfigMapKeyRef not Secret",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "FOO",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "k"},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container envFrom secretRef match",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "container envFrom secretRef mismatch",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container envFrom with ConfigMapRef not Secret",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "initContainer env secretKeyRef match",
			podSpec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
						Env: []corev1.EnvVar{
							{
								Name: "INIT_TOKEN",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "k"},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "initContainer envFrom secretRef match",
			podSpec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "ephemeralContainer env secretKeyRef match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							Env: []corev1.EnvVar{
								{
									Name: "DBG",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "k"},
									},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "ephemeralContainer envFrom secretRef match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "ephemeralContainer no match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "multiple containers second matches",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "a", Env: []corev1.EnvVar{{Name: "X", Value: "y"}}},
					{
						Name: "b",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "case sensitive secret name",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "v",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "My-Secret"},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "empty secretName never matches populated volume",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "v",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"},
						},
					},
				},
			},
			secretName: "",
			want:       false,
		},
		{
			name: "volume with Secret nil does not panic and returns false",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "cfg",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resources.PodSpecReferencesSecret(tt.podSpec, tt.secretName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPodSpecReferencesConfigMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		podSpec       *corev1.PodSpec
		configMapName string
		want          bool
	}{
		{
			name:          "nil PodSpec",
			podSpec:       nil,
			configMapName: "my-cm",
			want:          false,
		},
		{
			name:          "empty PodSpec",
			podSpec:       &corev1.PodSpec{},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "volume configMap direct match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "cm-vol",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "volume configMap mismatch",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "cm-vol",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "other-cm"}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "projected volume configMap match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "projected volume configMap mismatch",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "projected mixed second source matches",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "proj",
						VolumeSource: corev1.VolumeSource{
							Projected: &corev1.ProjectedVolumeSource{
								Sources: []corev1.VolumeProjection{
									{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "sec-a"}}},
									{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "container env configMapKeyRef match",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "FOO",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "key"},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "container env configMapKeyRef mismatch",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "FOO",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "other-cm"}, Key: "key"},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "container env with secretKeyRef not ConfigMap",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "FOO",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "k"},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "container envFrom configMapRef match",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "container envFrom configMapRef mismatch",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "container envFrom SecretRef not ConfigMap",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						EnvFrom: []corev1.EnvFromSource{
							{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "initContainer env configMapKeyRef match",
			podSpec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
						Env: []corev1.EnvVar{
							{
								Name: "INIT_FOO",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "k"},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "initContainer envFrom configMapRef match",
			podSpec: &corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name: "init",
						EnvFrom: []corev1.EnvFromSource{
							{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "ephemeralContainer env configMapKeyRef match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							Env: []corev1.EnvVar{
								{
									Name: "DBG",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}, Key: "k"},
									},
								},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "ephemeralContainer envFrom configMapRef match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "ephemeralContainer no match",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{
						EphemeralContainerCommon: corev1.EphemeralContainerCommon{
							Name: "debug",
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}}},
							},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "multiple volumes first matches",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "v1",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}},
						},
					},
					{
						Name: "v2",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "sec"},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "case sensitive configmap name",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "v",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "My-CM"}},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "empty configMapName never matches",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "v",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}},
						},
					},
				},
			},
			configMapName: "",
			want:          false,
		},
		{
			name: "volume with Secret not ConfigMap returns false",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "sec",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: "my-cm"},
						},
					},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resources.PodSpecReferencesConfigMap(tt.podSpec, tt.configMapName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPodSpecReferencesCrossTypeIsolation(t *testing.T) {
	t.Parallel()

	// Secret reference must not be reported as ConfigMap and vice-versa.
	secretPodSpec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "v",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: "shared-name"},
				},
			},
		},
	}

	assert.True(t, resources.PodSpecReferencesSecret(secretPodSpec, "shared-name"))
	assert.False(t, resources.PodSpecReferencesConfigMap(secretPodSpec, "shared-name"))

	cmPodSpec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "v",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "shared-name"}},
				},
			},
		},
	}

	assert.True(t, resources.PodSpecReferencesConfigMap(cmPodSpec, "shared-name"))
	assert.False(t, resources.PodSpecReferencesSecret(cmPodSpec, "shared-name"))
}

func TestPodSpecReferencesSecret_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		podSpec    *corev1.PodSpec
		secretName string
		want       bool
	}{
		{
			name: "empty secretName with empty SecretName volume currently matches (documents peculiar gap)",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ""}}},
				},
			},
			secretName: "",
			// Current implementation returns true because "" == "". Ideal should be false.
			// This test documents the peculiarity; if fixed, change expect to false.
			want: true,
		},
		{
			name: "volume SecretName empty vs non-empty target",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: ""}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "ConfigMap volume with same name does not match Secret",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "projected volume Sources empty slice",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{}}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "projected volume Sources nil slice",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: nil}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "projected with Secret nil but ConfigMap present does not match Secret",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}}}}}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "trailing space in secretName does not match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"}}},
				},
			},
			secretName: "my-secret ",
			want:       false,
		},
		{
			name: "name with slash does not falsely match prefix",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "my-secret/sub"}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "container Env with SecretKeyRef empty name vs target empty",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Env: []corev1.EnvVar{{Name: "X", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: ""}, Key: "k"}}}}},
				},
			},
			secretName: "",
			want:       true,
		},
		{
			name: "container Env ValueFrom with both SecretKeyRef and ConfigMapKeyRef set - secret matches",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						Env: []corev1.EnvVar{
							{
								Name: "X",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef:    &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "k"},
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "other"}, Key: "k"},
								},
							},
						},
					},
				},
			},
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "envFrom with SecretRef empty name vs non-empty target",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: ""}}}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "many volumes 99 non-matching plus 1 matching at end",
			podSpec: func() *corev1.PodSpec {
				vols := make([]corev1.Volume, 100)
				for i := 0; i < 99; i++ {
					vols[i] = corev1.Volume{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "other"}}}
				}
				vols[99] = corev1.Volume{Name: "match", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"}}}
				return &corev1.PodSpec{Volumes: vols}
			}(),
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "many containers with nil Env/EnvFrom slices still scans correctly",
			podSpec: func() *corev1.PodSpec {
				containers := make([]corev1.Container, 50)
				for i := range containers {
					containers[i] = corev1.Container{Name: "c"}
				}
				containers[49].Env = []corev1.EnvVar{{Name: "X", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"}, Key: "k"}}}}
				return &corev1.PodSpec{Containers: containers}
			}(),
			secretName: "my-secret",
			want:       true,
		},
		{
			name: "projected ServiceAccountToken source does not match",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{Path: "token"}}}}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "hostPath volume never matches Secret",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "host", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/tmp"}}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
		{
			name: "ephemeralContainer with nil Env and SecretRef in EnvFrom still not panic",
			podSpec: &corev1.PodSpec{
				EphemeralContainers: []corev1.EphemeralContainer{
					{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", Env: nil, EnvFrom: nil}},
				},
			},
			secretName: "my-secret",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resources.PodSpecReferencesSecret(tt.podSpec, tt.secretName)
			assert.Equal(t, tt.want, got, "PodSpecReferencesSecret")
		})
	}
}

func TestPodSpecReferencesConfigMap_PeculiarEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		podSpec       *corev1.PodSpec
		configMapName string
		want          bool
	}{
		{
			name: "empty configMapName with empty ConfigMap volume currently matches (peculiar gap)",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: ""}}}},
				},
			},
			configMapName: "",
			want:          true,
		},
		{
			name: "projected Sources nil vs empty both false",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: nil}}},
					{Name: "proj2", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{}}}},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "ConfigMap name with hyphen vs underscore mismatch",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my_cm"}}}},
				},
			},
			configMapName: "my-cm",
			want:          false,
		},
		{
			name: "Env ConfigMapKeyRef empty name vs target empty matches",
			podSpec: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Env: []corev1.EnvVar{{Name: "X", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: ""}, Key: "k"}}}}},
				},
			},
			configMapName: "",
			want:          true,
		},
		{
			name: "DownwardAPI + ConfigMap mixed projected only ConfigMap matches",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "proj", VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{{DownwardAPI: &corev1.DownwardAPIProjection{}}, {ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "my-cm"}}}}}}},
				},
			},
			configMapName: "my-cm",
			want:          true,
		},
		{
			name: "secret volume with same logical name does not count as ConfigMap",
			podSpec: &corev1.PodSpec{
				Volumes: []corev1.Volume{
					{Name: "v", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "shared"}}},
				},
			},
			configMapName: "shared",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := resources.PodSpecReferencesConfigMap(tt.podSpec, tt.configMapName)
			assert.Equal(t, tt.want, got)
		})
	}
}
