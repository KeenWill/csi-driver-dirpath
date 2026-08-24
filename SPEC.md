# csi-driver-dirpath — specification

A minimal CSI driver that provisions **per-volume directories on a pre-existing mounted filesystem** and bind-mounts them into pods. It exists to replace helper-pod-based local provisioners (e.g. local-path-provisioner) where volume setup latency matters: no pod is created per volume — the resident node plugin performs a `mkdir` and a bind mount, in milliseconds.

## Goals

- Dynamic provisioning of directory-backed volumes on a configured `basePath` (an existing mount the operator provides).
- Volume setup/teardown as local syscalls by a node-resident plugin. No helper pods, no per-volume scheduling round trips.
- Correct topology: a volume lives on one node; pods using it schedule there.
- Optional per-volume capacity enforcement via **XFS project quotas**.
- Small enough to audit in one sitting.

## Non-goals

- Creating, attaching, or formatting the base mount. The operator provides a mounted filesystem; this driver only consumes it.
- Loop devices, mkfs, raw block mode, snapshots, cross-node RWX.
- Being a general-purpose storage system. This is scratch/ephemeral-grade local storage with PVC lifecycle semantics.

## Architecture

One DaemonSet per node runs three containers:

1. **dirpath-plugin** — the driver: CSI Identity, Controller, and Node services over a unix socket.
2. **node-driver-registrar** (upstream sidecar) — registers the plugin with kubelet.
3. **external-provisioner** (upstream sidecar) in **distributed provisioning mode** (`--node-deployment`) — each node's provisioner handles `CreateVolume`/`DeleteVolume` for volumes on that node.

Distributed provisioning removes the need for any central controller Deployment and — critically — runs `DeleteVolume` on the node that owns the directory, so deletion is a local `rm -rf`. Leadership per PVC is handled by the sidecar's existing lease mechanism.

- **StorageClass**: `volumeBindingMode: WaitForFirstConsumer`, `reclaimPolicy: Delete`; parameters: `quotas` (`"true"`/`"false"`, default false). `basePath` is plugin configuration (per-DaemonSet), not a StorageClass parameter, so a compromised namespace cannot point volumes at arbitrary host paths.
- **Topology**: node plugin reports `topology.kubernetes.io/hostname`-style topology (`csi.dirpath.dev/node`); PVs carry node affinity for their node.

## Volume lifecycle

- `CreateVolume` (on the selected node, via distributed provisioning): validate the fence (below), `mkdir basePath/volumes/<volume-id>`, apply quota if enabled, return volume with node topology. Idempotent: an existing directory for the same volume id is success.
- `NodePublishVolume`: verify fence, bind-mount `basePath/volumes/<volume-id>` onto the target path. Idempotent; supports `fsGroup` via `fsGroupPolicy: File`.
- `NodeUnpublishVolume`: unmount the target. Idempotent — already-unmounted is success.
- `DeleteVolume`: remove the quota project (if any), `rm -rf` the directory. Idempotent.
- `NodeGetVolumeStats`: report usage/capacity — from project quota accounting when quotas are on, else `statfs` of the base filesystem.

### Reconciliation

On startup and periodically, the plugin lists `basePath/volumes/*` and compares against extant PVs of this driver on this node:

- Directory with no PV → orphan. After a configurable grace period, delete (default on; `--orphan-reclaim=false` to disable).
- PV with no directory → recreate empty on next publish attempt (data is gone; this is scratch storage — log loudly).

## Mount fence

The driver must never operate against an absent or wrong mount (e.g. the empty mountpoint directory on the root disk after a node reboot ordering issue).

- Default fence: a marker file `basePath/.dirpath-fence` whose content must equal the configured fence token. The operator creates it once when preparing the mount.
- Strict mode (optional): additionally pin the filesystem's fsid (`statfs f_fsid`) or the backing device `major:minor`.
- Enforcement: `Probe` fails (plugin reports unready, provisioning halts on that node) and every Controller/Node operation re-checks before touching the filesystem. Fence failure is a hard refusal, never a fallback path.

## XFS project quotas (optional)

Enabled per StorageClass (`quotas: "true"`), enforced per volume:

- Cap = the PVC's `resources.requests.storage`.
- The plugin allocates a unique project ID per volume (persistent mapping in `basePath/.dirpath-meta/projects`, project ID space partitioned per node), assigns the directory to the project, and sets a hard block limit (`xfs_quota` semantics via syscalls, not by shelling out, if practical — `FS_IOC_FSSETXATTR` + `Q_XSETPQLIM`).
- Prerequisite: `basePath` is XFS mounted with `prjquota`. When a quota-enabled StorageClass is used, `Probe`/`CreateVolume` verify this (mount flags via `/proc/self/mounts` + a quota-state query) and refuse otherwise with a clear error.
- Exceeding the cap surfaces as `ENOSPC` inside the pod. `NodeGetVolumeStats` reports quota-accounted usage so kubelet metrics are per-volume truthful.
- `ControllerExpandVolume` (optional, phase 2): with quotas on, expansion is just raising the limit — cheap to support, gated behind a capability flag.

Without quotas, volumes are unbounded shared-filesystem directories (documented plainly).

## Packaging & release

- **Helm chart** in `charts/csi-driver-dirpath` — the primary install path. Values: image, basePath, fence token/mode, orphan reclaim, sidecar versions, resources, StorageClass creation toggles.
- Container image and chart published to **ghcr.io** (chart as OCI artifact) by a GitHub Actions release workflow on tags.
- `deploy/` holds rendered plain manifests for kubectl-only consumers, regenerated from the chart in CI.

## Implementation notes

- Go; `github.com/container-storage-interface/spec` for the gRPC API; kubernetes-csi sidecars pinned by digest.
- Privileged node plugin (needs mount syscalls + rshared propagation to `basePath` and kubelet dirs); RBAC limited to what distributed provisioning requires (PV/PVC/StorageClass/Node read-write-as-needed + leases).
- Logging structured; Prometheus metrics endpoint with provision/publish latency histograms and (when quotas on) per-volume usage.

## Testing

- `csi-sanity` against the driver in CI.
- kind-based e2e: provision → publish → write → unpublish → delete; orphan reconciliation; fence-failure refusal.
- Quota e2e on an XFS loopback image fixture (created in CI; no special runners needed): cap enforcement (`ENOSPC`), stats accuracy, expansion if implemented.

## Milestones

1. Skeleton: Identity/Node/Controller no-op services, sanity harness green.
2. Directory lifecycle end-to-end in kind (no quotas), fence + reconciliation.
3. Helm chart + ghcr publishing workflow.
4. XFS project quotas + loopback e2e.
5. (Optional) expansion, metrics polish.
