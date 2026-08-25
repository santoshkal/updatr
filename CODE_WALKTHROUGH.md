# Code Walkthrough — updatr

This document traces the **DATA-PATH** from the process entrypoint through every component that participates in a restart. All paths are the same for `Secret` and `ConfigMap` except the top-level type.

```
main.go  ->  predicate  ->  controller.Reconcile  ->  resources  ->  rollout
```

---

## 1. Entrypoint — `main.go:1`

### 1.1 Scheme registration — `main.go:28`

```go
func init() {
    utilruntime.Must(clientgoscheme.AddToScheme(scheme)) // k8s core types
    utilruntime.Must(corev1.AddToScheme(scheme))         // Secret, ConfigMap, Pod
    utilruntime.Must(appsv1.AddToScheme(scheme))         // Deployment, StatefulSet
}
```

- `runtime.NewScheme()` (`main.go:23`) creates the global `scheme`.
- `init()` registers three groups before `main()` runs. Every `client.Get`/`List`/`Patch` later uses this scheme to decode API objects. Panic on failure via `utilruntime.Must()` surfaces mis-registration immediately.

### 1.2 Flag parsing & logger — `main.go:37`

```
flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", ...)
flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", ...)
flag.BoolVar(&enableLeaderElection, "leader-elect", false, ...)
opts := zap.Options{Development: true}
opts.BindFlags(flag.CommandLine)
flag.Parse()
ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
```

- `flag.StringVar`/`BoolVar` (`main.go:44`) define CLI flags.
- `zap.Options{Development: true}` (`main.go:68`) enables human-readable dev logging.
- `opts.BindFlags()` (`main.go:72`) exposes `--zap-log-level`, `--zap-encoder`, etc.
- `flag.Parse()` (`main.go:74`) materializes flags.
- `zap.New(zap.UseFlagOptions(&opts))` (`main.go:79`) builds the logger; `ctrl.SetLogger()` installs it globally so `log.FromContext(ctx)` in reconcilers inherits it.

### 1.3 Manager creation — `main.go:82`

```go
kubeConfig := ctrl.GetConfigOrDie() // KUBECONFIG or in-cluster
mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
    Scheme:                 scheme,
    Metrics:                metricsserver.Options{BindAddress: metricsAddr},
    HealthProbeBindAddress: probeAddr,
    LeaderElection:         enableLeaderElection,
    LeaderElectionID:       "updatr.santoshkal.github.io",
})
```

- `ctrl.GetConfigOrDie()` (`main.go:82`) loads kubeconfig.
- `ctrl.NewManager()` (`main.go:85`) builds the controller-runtime `Manager`:
  - starts a shared informer cache (watches Secrets, ConfigMaps, Deployments, StatefulSets),
  - provides `mgr.GetClient()` (typed `client.Client` with cache read, `Patch`/`Get`/`List`),
  - serves metrics (`:8080`) and health probes (`:8081`),
  - optionally enables leader election for HA.

### 1.4 Controller registration — `main.go:101`

```go
secretReconciler := &controller.SecretReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
secretReconciler.SetupWithManager(mgr)

cmReconciler := &controller.ConfigMapReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
cmReconciler.SetupWithManager(mgr)
```

- Both reconcilers receive `mgr.GetClient()` (`main.go:103`) — the cached client used in `Reconcile` for `Get`/`List`/`Patch`.
- `SetupWithManager` (`main.go:106`) registers each reconciler with the manager's controller builder (see §3).

### 1.5 Health probes & start — `main.go:127`

```go
mgr.AddHealthzCheck("healthz", healthz.Ping)
mgr.AddReadyzCheck("readyz", healthz.Ping)
setupLog.Info("starting manager")
mgr.Start(ctrl.SetupSignalHandler())
```

- `AddHealthzCheck`/`AddReadyzCheck` (`main.go:128`) expose `/healthz` and `/readyz`.
- `mgr.Start(ctrl.SetupSignalHandler())` (`main.go:146`) blocks: starts caches, waits for cache sync, starts controllers, handles `SIGINT`/`SIGTERM` via context cancellation. `log.FromContext(ctx)` in reconcilers derives from this context.

---

## 2. Event filter — `internal/predicate/resourceversion.go:16`

### 2.1 Purpose

The informer cache emits `Create`/`Update`/`Delete`/`Generic` events for every object. Without filtering, periodic resyncs and `kubectl get --watch` re-lists would trigger reconciles even when nothing changed. The project requirement: *no unwanted reconciles when `resourceVersion` is not updated*.

### 2.2 Implementation — `ResourceVersionChanged:16`

```go
func ResourceVersionChanged() predicate.Predicate {
    return predicate.Funcs{
        CreateFunc:  func(_ event.CreateEvent) bool { return false },
        DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
        GenericFunc: func(_ event.GenericEvent) bool { return false },
        UpdateFunc: func(e event.UpdateEvent) bool {
            oldRV := e.ObjectOld.GetResourceVersion() // "12345"
            newRV := e.ObjectNew.GetResourceVersion() // "12346"
            return oldRV != newRV
        },
    }
}
```

- `predicate.Funcs` (`internal/predicate/resourceversion.go:19`) builds the filter.
- `Create/Delete/Generic -> false` (`resourceversion.go:20`) — ignore creation/deletion; a new `Secret` with no consumer needs no rollout, a deleted one will be handled by the workload's own error.
- `UpdateFunc` (`resourceversion.go:29`) compares `GetResourceVersion()` on old vs new object. `resourceVersion` is an opaque etcd monotonic string bumped by the API server on every persisted write. Only `oldRV != newRV` enqueues the `NamespacedName` for `Reconcile`.
- Edge: `nil` guard (`resourceversion.go:30`) protects against cache edge cases.
- Note: cluster restarts can also bump RV; the reconcile is idempotent (re-patching the timestamp is harmless, pods already restarted).

---

## 3. Controller wiring — `internal/controller/secret_controller.go:128` and `configmap_controller.go:128`

Both `SetupWithManager` methods are identical in shape:

```go
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
    rvPredicate := pred.ResourceVersionChanged() // build RV filter
    return ctrl.NewControllerManagedBy(mgr).
        For(&corev1.Secret{}).          // primary watch: Secret (or ConfigMap)
        WithEventFilter(rvPredicate).    // only RV-change updates pass
        Complete(r)                      // r.Reconcile becomes the handler
}
```

- `ctrl.NewControllerManagedBy(mgr)` (`secret_controller.go:135`) creates a controller builder.
- `.For(&corev1.Secret{})` (`secret_controller.go:136`) tells the cache to `Watch` Secrets with `List`+`Watch`.
- `.WithEventFilter(rvPredicate)` (`secret_controller.go:137`) installs the filter from §2.
- `.Complete(r)` (`secret_controller.go:138`) registers `Reconcile` and starts the worker.

`SecretReconciler` (`secret_controller.go:23`) and `ConfigMapReconciler` (`configmap_controller.go:23`) are otherwise independent; they share the same `rollout` and `resources` helpers.

---

## 4. Reconcile DATA-PATH — `internal/controller/secret_controller.go:33` (ConfigMap: `configmap_controller.go:33`)

Trace a single `Secret` update `rv 1001 -> 1002` in namespace `default`, name `db-creds`:

### 4.1 Enqueue

1. `kube-apiserver` persists `Secret` change -> new `resourceVersion=1002`.
2. Informer cache sees `Update(oldRV=1001, newRV=1002)`.
3. `ResourceVersionChanged.UpdateFunc` returns `true` (1001 != 1002).
4. Controller workqueue enqueues `ctrl.Request{NamespacedName: {Namespace:"default", Name:"db-creds"}}`.

### 4.2 `Reconcile(ctx, req)` — `secret_controller.go:33`

```go
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    logger.Info("reconciling Secret", "secret", req.NamespacedName)

    var secret corev1.Secret
    if err := r.Get(ctx, req.NamespacedName, &secret); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err) // deleted -> stop
    }

    if err := r.restartDeploymentsForSecret(ctx, &secret); err != nil {
        return ctrl.Result{}, fmt.Errorf("restarting deployments for secret %q: %w", req.NamespacedName, err)
    }
    if err := r.restartStatefulSetsForSecret(ctx, &secret); err != nil {
        return ctrl.Result{}, fmt.Errorf("restarting statefulsets for secret %q: %w", req.NamespacedName, err)
    }
    return ctrl.Result{}, nil // no requeue
}
```

- `log.FromContext(ctx)` (`secret_controller.go:38`) fetches the request-scoped logger (fields: controller, name, namespace).
- `r.Get(ctx, req.NamespacedName, &secret)` (`secret_controller.go:44`) reads the `Secret` from the cache (or API server if not cached). `client.IgnoreNotFound` (`secret_controller.go:46`) suppresses the requeue if the object was deleted between enqueue and reconcile.
- Delegates to two helpers that cover both workload kinds; errors are wrapped with `%q` and `%w` (`secret_controller.go:52`) for visible boundaries and `errors.Is/As` unwrapping.

`ConfigMapReconciler.Reconcile` (`configmap_controller.go:33`) is identical, operating on `corev1.ConfigMap`.

### 4.3 Listing consumers — `secret_controller.go:65` / `configmap_controller.go:65`

```go
func (r *SecretReconciler) restartDeploymentsForSecret(ctx context.Context, secret *corev1.Secret) error {
    var list appsv1.DeploymentList
    if err := r.List(ctx, &list, client.InNamespace(secret.Namespace)); err != nil {
        return fmt.Errorf("listing deployments in namespace %q: %w", secret.Namespace, err)
    }
    for i := range list.Items {
        dep := &list.Items[i]
        if !resources.PodSpecReferencesSecret(&dep.Spec.Template.Spec, secret.Name) {
            continue
        }
        if err := triggerDeploymentRollout(ctx, r.Client, dep); err != nil {
            return fmt.Errorf("patching deployment %q/%q: %w", dep.Namespace, dep.Name, err)
        }
    }
    return nil
}
```

- `r.List(ctx, &list, client.InNamespace(secret.Namespace))` (`secret_controller.go:71`) lists all `Deployments` in the **same namespace** as the `Secret` — cross-namespace references are not possible for these volume/env fields.
- `for i := range list.Items { dep := &list.Items[i] }` (`secret_controller.go:77`) uses index-range to take a stable pointer to the slice element (avoids loop-var aliasing).
- `resources.PodSpecReferencesSecret(&dep.Spec.Template.Spec, secret.Name)` (`secret_controller.go:81`) — reference check (see §5). `continue` skips non-consumers.
- `triggerDeploymentRollout(ctx, r.Client, dep)` (`secret_controller.go:86`) — patch on match.

`restartStatefulSetsForSecret` (`secret_controller.go:95`) and the ConfigMap variants (`configmap_controller.go:65`, `configmap_controller.go:95`) mirror this for `StatefulSetList`.

Data size: `PodSpec` passed as `*corev1.PodSpec` (`resources/references.go:12`) to avoid copying the large struct on a hot listing path.

---

## 5. Reference detection — `internal/resources/references.go:12`

### 5.1 Entry — `PodSpecReferencesSecret` / `PodSpecReferencesConfigMap`

```go
func PodSpecReferencesSecret(podSpec *corev1.PodSpec, secretName string) bool {
    if podSpec == nil { return false }
    if referencesSecretInVolumes(podSpec.Volumes, secretName) { return true }
    if referencesSecretInContainers(podSpec.Containers, secretName) { return true }
    if referencesSecretInContainers(podSpec.InitContainers, secretName) { return true }
    if referencesSecretInEphemeralContainers(podSpec.EphemeralContainers, secretName) { return true }
    return false
}
```

- Short-circuit OR chain (`references.go:17`): return `true` on first match; `false` only if all checks miss.

### 5.2 Volume sources — `referencesSecretInVolumes:70`

- `vol.Secret.SecretName == name` (`references.go:73`) — `volumes[].secret.secretName`.
- `vol.Projected.Sources[].Secret.Name == name` (`references.go:84`) — projected volumes. Nil guard on `vol.Projected` uses early `continue` (`references.go:78`) to reduce nesting.
- ConfigMap variant checks `vol.ConfigMap.Name` and `projected.sources[].configMap.name` (`references.go:93`).

### 5.3 Env sources — `referencesSecretInContainers:116`

```go
for _, c := range containers {
    for _, env := range c.Env {
        hasSecretKeyRef := env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil
        isTargetSecret := hasSecretKeyRef && env.ValueFrom.SecretKeyRef.Name == name
        if isTargetSecret { return true }
    }
    for _, envFrom := range c.EnvFrom {
        if envFrom.SecretRef != nil && envFrom.SecretRef.Name == name { return true }
    }
}
```

- `Env.ValueFrom.SecretKeyRef.Name` (`references.go:122`) — `env.valueFrom.secretKeyRef`.
- `EnvFrom.SecretRef.Name` (`references.go:133`) — `envFrom.secretRef`.
- Named booleans `hasSecretKeyRef`/`isTargetSecret` (`references.go:122`) satisfy the `3+ operands must be named` style rule and avoid repeating the 3-operand wall.
- Ephemeral containers repeat the same logic with `[]corev1.EphemeralContainer` (`references.go:166`).

All four container slices are checked, so sidecars, init, and ephemeral debug containers are covered.

---

## 6. Rollout trigger — `internal/controller/rollout.go:23`

### 6.1 Why an annotation patch

Kubernetes restarts workloads when `spec.template` changes (the Deployment controller hashes the PodTemplate and creates a new ReplicaSet). Patching an annotation is the idiomatic `kubectl rollout restart` mechanism — minimal, no spec drift, and `time.RFC3339Nano` guarantees uniqueness.

### 6.2 Deployment path — `triggerDeploymentRollout:23`

```go
func triggerDeploymentRollout(ctx context.Context, c client.Client, deployment *appsv1.Deployment) error {
    logger := log.FromContext(ctx)
    original := deployment.DeepCopy() // snapshot for MergeFrom diff

    if deployment.Spec.Template.Annotations == nil {
        deployment.Spec.Template.Annotations = make(map[string]string, 1) // avoid nil panic
    }
    now := time.Now().UTC().Format(time.RFC3339Nano)
    deployment.Spec.Template.Annotations[RestartAnnotation] = now // "updatr.github.com/restartedAt"

    patch := client.MergeFrom(original) // build JSON merge patch from diff
    if err := c.Patch(ctx, deployment, patch); err != nil {
        return fmt.Errorf("patch deployment %q: %w", types.NamespacedName{...}, err)
    }
    logger.Info("triggered deployment rollout", "deployment", types.NamespacedName{...}, "annotation", RestartAnnotation)
    return nil
}
```

- `deployment.DeepCopy()` (`rollout.go:31`) snapshots before mutation.
- `make(map[string]string, 1)` (`rollout.go:36`) explicit non-nil map (prevents `null` vs `[]` style issues, avoids panic on write).
- `time.Now().UTC().Format(time.RFC3339Nano)` (`rollout.go:40`) produces the annotation value.
- `client.MergeFrom(original)` (`rollout.go:45`) creates a `MergePatch` diffing `original` vs mutated object.
- `c.Patch(ctx, deployment, patch)` (`rollout.go:47`) sends `PATCH` to the API server; error wrapped with `%q`/`%w` (`rollout.go:49`) for visible boundaries and `errors.Is/As`.
- `logger.Info` (`rollout.go:60`) records the rollout with structured fields.

### 6.3 StatefulSet path — `triggerStatefulSetRollout:74`

Identical, operating on `*appsv1.StatefulSet` (`rollout.go:74`). `StatefulSet` also rolls on PodTemplate change; the same annotation patch triggers the `StatefulSet` controller's ordered rolling update.

Error wrapping mirrors Deployment (`rollout.go:101`). Both functions use `log.FromContext(ctx)` (`rollout.go:30`) so every rollout log is correlated to the reconcile request.

---

## 7. End-to-end sequence (ConfigMap example)

1. `kubectl patch configmap/app-config --patch '{"data":{"FEATURE":"on"}}'` -> apiserver bumps `resourceVersion` `500 -> 501`.
2. Manager's `ConfigMap` informer cache sees `Update{500,501}`.
3. `ResourceVersionChanged.UpdateFunc` (`predicate/resourceversion.go:29`) `500 != 501` -> `true` -> enqueue `default/app-config`.
4. Manager's worker calls `ConfigMapReconciler.Reconcile(ctx, req)` (`configmap_controller.go:33`).
5. `r.Get` fetches `app-config` (`configmap_controller.go:43`).
6. `restartDeploymentsForConfigMap` lists `DeploymentList` in `default` (`configmap_controller.go:71`).
7. For each `dep`, `PodSpecReferencesConfigMap(&dep.Spec.Template.Spec, "app-config")` (`configmap_controller.go:81`) checks volumes/env. Suppose `deploy/api` mounts it via `envFrom.configMapRef`.
8. `triggerDeploymentRollout(ctx, client, deploy/api)` (`configmap_controller.go:86`) patches `spec.template.annotations["updatr.github.com/restartedAt"]="2026-08-25T12:00:00.123456789Z"`.
9. API server persists; Deployment controller hashes new PodTemplate, creates `ReplicaSet api-xyz`, scales down old, pods restart with new ConfigMap data.
10. Same for `StatefulSet` consumers in that namespace via `restartStatefulSetsForConfigMap` (`configmap_controller.go:95`).
11. `Reconcile` returns `ctrl.Result{}, nil` — no requeue.

If the `UpdateEvent` had `500 -> 500` (resync, annotation-only status update, etc.), `ResourceVersionChanged` returns `false` and no `Reconcile` is enqueued — the DATA-PATH stops at §2.

---

## 8. Component dependency graph

```
main.go
 ├─ schemes: client-go/corev1/appsv1
 ├─ manager (cache + client + metrics + health + leader election)
 ├─ SecretReconciler ─┬─ predicate.ResourceVersionChanged
 │                    ├─ client.Get/List/Patch (via manager cache)
 │                    ├─ resources.PodSpecReferencesSecret
 │                    └─ rollout.triggerDeploymentRollout / triggerStatefulSetRollout
 └─ ConfigMapReconciler ─┬─ predicate.ResourceVersionChanged
                         ├─ client.Get/List/Patch
                         ├─ resources.PodSpecReferencesConfigMap
                         └─ rollout.* (shared)
```

No CRDs, no webhooks, no owner refs — just `Watch` + `Predicate` + `Patch`.

---

## 9. Files

| Path | Role | Lines |
|---|---|---|
| `main.go:1` | Manager bootstrap, flag/logger/health | 152 |
| `internal/predicate/resourceversion.go:1` | RV-change filter | 43 |
| `internal/resources/references.go:1` | PodSpec reference inspection | 214 |
| `internal/controller/rollout.go:1` | Annotation patch to trigger rollout | 124 |
| `internal/controller/secret_controller.go:1` | Secret watch + reconcile | 139 |
| `internal/controller/configmap_controller.go:1` | ConfigMap watch + reconcile | 139 |
| `go.mod:1` | `module github.com/santoshkal/updatr`, `go 1.26.6` | 64 |
