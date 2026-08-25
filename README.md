# csi-driver-dirpath
Minimal CSI driver that provisions per-volume directories on an existing node mount via bind-mounts. DaemonSet node plugin, no helper pods, optional XFS project quotas.

**Status:** milestones 1 through 3 are implemented: the CSI services pass `csi-sanity`; the unquotaed directory lifecycle, mount fence, distributed provisioning, and orphan reconciliation run end-to-end in kind; and the driver ships as an image, Helm chart, and rendered manifests. XFS project quotas remain a future milestone. See [SPEC.md](SPEC.md) for the full design.

## Why

Helper-pod-based local provisioners (e.g. local-path-provisioner) pay a full pod schedule/start/observe/delete round trip per volume, serialized through one controller — under bursty churn (CI runners), volume creation queues for minutes. A node-resident CSI plugin does the same work as one `mkdir` and one bind mount.

## Install

Prepare the configured base path on every target node and write the exact fence token to `<basePath>/.dirpath-fence`. The default base path is `/var/lib/dirpath`.

Install the OCI chart and optionally create its StorageClass:

```sh
helm install csi-driver-dirpath oci://ghcr.io/keenwill/csi-driver-dirpath \
  --version v0.1.0 \
  --namespace kube-system \
  --set-file fence.token=/path/to/fence-token \
  --set storageClass.create=true
```

The chart also supports an existing Secret through `fence.existingSecret`, strict `fsid` or `device` fence modes, custom base paths and images, and per-container resource settings. Quotas must remain disabled until milestone 4.

Kubectl-only consumers can edit the fence token in `deploy/csi-driver-dirpath.yaml` and apply that rendered manifest. Maintainers regenerate both plain manifests with `make manifests` after chart changes.

## License

Apache-2.0
