# updatr

A Kubernetes controller that restarts `Deployment` and `StatefulSet` workloads when a `Secret` or `ConfigMap` they consume changes.

`updatr` watches every `Secret` and `ConfigMap` in the cluster and, on an observed `resourceVersion` change, finds the workloads in the same namespace whose `PodTemplate` references that object (via `env`, `envFrom`, `volumes`, or `projected` volumes) and triggers a rolling restart by patching the PodTemplate annotation `updatr.github.com/restartedAt`.

> **Domain:** `github.com/santoshkal/updatr` | **Go:** `1.26.6` | **Kubernetes:** `v1.36.x` (`k8s.io/api@v0.36.4`) | **controller-runtime:** `v0.24.1`

## Why updatr?

Kubernetes does not automatically roll workloads when a mounted `Secret`/`ConfigMap` changes — pods keep the old values (or kubelet eventually syncs volumes, but `env` never updates). `updatr` closes that gap: any data change bumps `resourceVersion`, the controller sees the bump and restarts consumers so new pods get the new data.

Key properties:

- **Zero-config** — no annotations on workloads required; reference detection is automatic.
- **Precise** — `predicate.ResourceVersionChanged` (`internal/predicate/resourceversion.go:16`) filters out resyncs, periodic cache re-lists, and events where `resourceVersion` is unchanged. No spurious reconciles.
- **Namespace-scoped effect** — only workloads in the same namespace as the changed `Secret`/`ConfigMap` are considered.

## How it works (30s)

```
Secret "db-creds" (rv=1001 -> rv=1002)
        │
        │ UpdateEvent (predicate allows it)
        ▼
SecretReconciler.Reconcile()  --  Get Secret, List Deployments/StatefulSets in namespace
        │
        ├─ resources.PodSpecReferencesSecret(&podSpec, "db-creds") ?
        │       checks volumes.secret, projected.sources[].secret,
        │              containers[].env.valueFrom.secretKeyRef,
        │              containers[].envFrom.secretRef (+ init/ephemeral)
        │
        └─ yes -> rollout.go:triggerDeploymentRollout() patches
                  spec.template.metadata.annotations["updatr.github.com/restartedAt"]=<now RFC3339Nano>
                  Deployment controller sees new PodTemplate hash -> new ReplicaSet -> rolling restart
```

Same path for `ConfigMap` via `ConfigMapReconciler` and `PodSpecReferencesConfigMap`.

## Supported references

All checked in `internal/resources/references.go:12`:

| Source | Field |
|---|---|
| volumes | `volume.secret.secretName`, `volume.configMap.name`, `volume.projected.sources[].secret.name`, `volume.projected.sources[].configMap.name` |
| env | `containers[].env[].valueFrom.secretKeyRef.name`, `containers[].env[].valueFrom.configMapKeyRef.name` |
| envFrom | `containers[].envFrom[].secretRef.name`, `containers[].envFrom[].configMapRef.name` |

Checked for `containers`, `initContainers`, and `ephemeralContainers`.

Workloads: `apps/v1` `Deployment` and `StatefulSet` (ReplicaSet-managed workloads — covers `Deployment`, and directly `StatefulSet`).

## Requirements

- Kubernetes `v1.36.x`
- Go `v1.26` for building

## Quick start

### Build locally

```bash
git clone https://github.com/santoshkal/updatr
cd updatr
go mod tidy          # Go 1.26.6, k8s.io/api v0.36.4, controller-runtime v0.24.1
go vet ./...
go build -o bin/updatr .
```

### Run out-of-cluster (against current kubeconfig)

```bash
./bin/updatr \
  --metrics-bind-address=:8080 \
  --health-probe-bind-address=:8081 \
  --leader-elect=false

# with zap verbosity:
./bin/updatr --zap-log-level=debug --zap-development=true
```

The manager uses `ctrl.GetConfigOrDie()` (`main.go:82`) — it respects `KUBECONFIG` or in-cluster config.

### Run in-cluster

Apply RBAC + Deployment (example, adjust namespace/image):

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: updatr
  namespace: updatr-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: updatr
rules:
  - apiGroups: [""]
    resources: ["secrets", "configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets"]
    verbs: ["get", "list", "watch", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: updatr
roleRef: { apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: updatr }
subjects:
  - kind: ServiceAccount; name: updatr; namespace: updatr-system
---
apiVersion: apps/v1
kind: Deployment
metadata: { name: updatr, namespace: updatr-system }
spec:
  replicas: 1
  selector: { matchLabels: { app: updatr } }
  template:
    metadata: { labels: { app: updatr } }
    spec:
      serviceAccountName: updatr
      containers:
        - name: manager
          image: ghcr.io/santoshkal/updatr:latest
          args: ["--leader-elect=true"]
          ports:
            - { containerPort: 8080, name: metrics }
            - { containerPort: 8081, name: health }
          livenessProbe: { httpGet: { path: /healthz, port: 8081 } }
          readinessProbe: { httpGet: { path: /readyz, port: 8081 } }
```

Leader election (`main.go:92` `LeaderElectionID: "updatr.santoshkal.github.io"`) is recommended when running >1 replica.

### Flags

| Flag | Default | Desc |
|---|---|---|
| `--metrics-bind-address` | `:8080` | Metrics endpoint |
| `--health-probe-bind-address` | `:8081` | `/healthz` and `/readyz` |
| `--leader-elect` | `false` | Enable leader election |
| `--zap-*` | — | Zap logger flags (via `zap.Options:main.go:68`) |

## Verification

```bash
# 1. Create a consumer
kubectl create configmap demo --from-literal=key=old
kubectl create deployment demo --image=nginx --dry-run=client -o yaml | \
  yq '.spec.template.spec.containers[0].envFrom[0].configMapRef.name="demo"' | \
  kubectl apply -f -

# 2. Note rollout annotation before
kubectl get deploy demo -o jsonpath='{.spec.template.metadata.annotations}'

# 3. Update the ConfigMap
kubectl patch configmap demo --patch '{"data":{"key":"new"}}'

# 4. Observe updatr log and annotation bump
kubectl logs deploy/updatr -n updatr-system
kubectl get deploy demo -o jsonpath='{.spec.template.metadata.annotations.updatr\.github\.com/restartedAt}'
# -> new RFC3339Nano timestamp

# Same for Secret:
kubectl create secret generic demo-sec --from-literal=tok=old
# (mount via volumes/env), then patch -> rollout
```

Cluster restarts that bump `resourceVersion` without data change also appear as an update; pods will already have restarted with the node, so the extra rollout is idempotent.

## Development

```bash
go fmt ./...
go vet ./...
go test ./...          # add envtest if you add integration tests
```

Project layout:

```
.
├── main.go                           # entrypoint, manager + controller wiring
├── go.mod                            # module github.com/santoshkal/updatr, go 1.26.6
├── internal/
│   ├── predicate/resourceversion.go  # RV-change filter
│   ├── resources/references.go       # PodSpec -> Secret/ConfigMap reference detection
│   └── controller/
│       ├── rollout.go                # patch PodTemplate annotation
│       ├── secret_controller.go      # SecretReconciler
│       └── configmap_controller.go   # ConfigMapReconciler
```

## Notes

- Only `Update` events where `oldRV != newRV` reconcile (`predicate.ResourceVersionChanged:internal/predicate/resourceversion.go:29`). `Create`/`Delete`/`Generic` are ignored.
- Patch is `client.MergeFrom` on the PodTemplate annotation (`internal/controller/rollout.go:44`), not a spec change — safe and minimal.
- No CRD; no workload annotation required.

## License

MIT (or as per repo). Contributions welcome — please run `go fmt`/`go vet` and keep each function call documented with inline comments (project convention).
