// Package resources inspects PodSpecs to determine Secret and ConfigMap usage.
package resources

import (
	corev1 "k8s.io/api/core/v1"
)

// PodSpecReferencesSecret reports whether podSpec references the Secret named secretName.
// It checks volumes, projected volumes, env valueFrom, and envFrom across all
// container types. PodSpec is taken as pointer to avoid copying the large
// struct (~300+ bytes) on each call on the hot listing path.
func PodSpecReferencesSecret(podSpec *corev1.PodSpec, secretName string) bool {
	if podSpec == nil {
		return false
	}

	// referencesSecretInVolumes() scans Volume sources for a matching Secret name.
	if referencesSecretInVolumes(podSpec.Volumes, secretName) {
		return true
	}

	// referencesSecretInContainers() checks env and envFrom in regular containers.
	if referencesSecretInContainers(podSpec.Containers, secretName) {
		return true
	}

	// referencesSecretInContainers() checks env and envFrom in init containers.
	if referencesSecretInContainers(podSpec.InitContainers, secretName) {
		return true
	}

	// referencesSecretInEphemeralContainers() checks ephemeral containers for Secret refs.
	if referencesSecretInEphemeralContainers(podSpec.EphemeralContainers, secretName) {
		return true
	}

	return false
}

// PodSpecReferencesConfigMap reports whether podSpec references the ConfigMap named configMapName.
// PodSpec is taken as pointer to avoid large-value copy.
func PodSpecReferencesConfigMap(podSpec *corev1.PodSpec, configMapName string) bool {
	if podSpec == nil {
		return false
	}

	// referencesConfigMapInVolumes() scans Volume sources for a matching ConfigMap name.
	if referencesConfigMapInVolumes(podSpec.Volumes, configMapName) {
		return true
	}

	// referencesConfigMapInContainers() checks env and envFrom in regular containers.
	if referencesConfigMapInContainers(podSpec.Containers, configMapName) {
		return true
	}

	// referencesConfigMapInContainers() checks env and envFrom in init containers.
	if referencesConfigMapInContainers(podSpec.InitContainers, configMapName) {
		return true
	}

	// referencesConfigMapInEphemeralContainers() checks ephemeral containers for ConfigMap refs.
	if referencesConfigMapInEphemeralContainers(podSpec.EphemeralContainers, configMapName) {
		return true
	}

	return false
}

func referencesSecretInVolumes(volumes []corev1.Volume, name string) bool {
	// range iterates over each Volume to inspect Secret and Projected sources.
	for _, vol := range volumes {
		if vol.Secret != nil && vol.Secret.SecretName == name {
			return true
		}

		// continue early if no projected sources to reduce nesting.
		if vol.Projected == nil {
			continue
		}

		// range iterates over projected sources (Secret/ConfigMap/DownwardAPI).
		for _, src := range vol.Projected.Sources {
			if src.Secret != nil && src.Secret.Name == name {
				return true
			}
		}
	}

	return false
}

func referencesConfigMapInVolumes(volumes []corev1.Volume, name string) bool {
	// range iterates over each Volume to inspect ConfigMap and Projected sources.
	for _, vol := range volumes {
		if vol.ConfigMap != nil && vol.ConfigMap.Name == name {
			return true
		}

		// continue early if no projected sources to reduce nesting.
		if vol.Projected == nil {
			continue
		}

		// range iterates over projected sources to find ConfigMap refs.
		for _, src := range vol.Projected.Sources {
			if src.ConfigMap != nil && src.ConfigMap.Name == name {
				return true
			}
		}
	}

	return false
}

func referencesSecretInContainers(containers []corev1.Container, name string) bool {
	// range iterates over containers to inspect Env and EnvFrom.
	for _, c := range containers {
		// range iterates over Env vars to find SecretKeyRef.
		for _, env := range c.Env {
			// hasSecretKeyRef checks the two nil guards as a named boolean for readability.
			hasSecretKeyRef := env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil
			// isTargetSecret combines the guard with the name match.
			isTargetSecret := hasSecretKeyRef && env.ValueFrom.SecretKeyRef.Name == name
			if isTargetSecret {
				return true
			}
		}

		// range iterates over EnvFrom to find SecretRef.
		for _, envFrom := range c.EnvFrom {
			if envFrom.SecretRef != nil && envFrom.SecretRef.Name == name {
				return true
			}
		}
	}

	return false
}

func referencesConfigMapInContainers(containers []corev1.Container, name string) bool {
	// range iterates over containers to inspect Env and EnvFrom for ConfigMap.
	for _, c := range containers {
		// range iterates over Env vars to find ConfigMapKeyRef.
		for _, env := range c.Env {
			// hasConfigMapKeyRef extracts the 3-operand condition into a named boolean.
			hasConfigMapKeyRef := env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil
			// isTargetConfigMap combines guard with name comparison.
			isTargetConfigMap := hasConfigMapKeyRef && env.ValueFrom.ConfigMapKeyRef.Name == name
			if isTargetConfigMap {
				return true
			}
		}

		// range iterates over EnvFrom to find ConfigMapRef.
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == name {
				return true
			}
		}
	}

	return false
}

func referencesSecretInEphemeralContainers(containers []corev1.EphemeralContainer, name string) bool {
	// range iterates over ephemeral containers to inspect Secret references.
	for _, c := range containers {
		// range iterates over Env vars to find SecretKeyRef.
		for _, env := range c.Env {
			// hasSecretKeyRef named boolean clarifies the 3-operand guard.
			hasSecretKeyRef := env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil
			// isTargetSecret is true when the Secret name matches.
			isTargetSecret := hasSecretKeyRef && env.ValueFrom.SecretKeyRef.Name == name
			if isTargetSecret {
				return true
			}
		}

		// range iterates over EnvFrom to find SecretRef.
		for _, envFrom := range c.EnvFrom {
			if envFrom.SecretRef != nil && envFrom.SecretRef.Name == name {
				return true
			}
		}
	}

	return false
}

func referencesConfigMapInEphemeralContainers(containers []corev1.EphemeralContainer, name string) bool {
	// range iterates over ephemeral containers to inspect ConfigMap references.
	for _, c := range containers {
		// range iterates over Env vars to find ConfigMapKeyRef.
		for _, env := range c.Env {
			// hasConfigMapKeyRef named boolean for 3-operand readability.
			hasConfigMapKeyRef := env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil
			// isTargetConfigMap checks the name after nil guards.
			isTargetConfigMap := hasConfigMapKeyRef && env.ValueFrom.ConfigMapKeyRef.Name == name
			if isTargetConfigMap {
				return true
			}
		}

		// range iterates over EnvFrom to find ConfigMapRef.
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == name {
				return true
			}
		}
	}

	return false
}
