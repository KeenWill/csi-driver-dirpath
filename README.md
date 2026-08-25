# csi-driver-dirpath
Minimal CSI driver that provisions per-volume directories on an existing node mount via bind-mounts. DaemonSet node plugin, no helper pods, optional XFS project quotas.

**Status:** milestones 1 and 2 are implemented: the CSI services pass `csi-sanity`, and the unquotaed directory lifecycle, mount fence, distributed provisioning, and orphan reconciliation run end-to-end in kind. XFS project quotas and release packaging remain future milestones. See [SPEC.md](SPEC.md) for the full design.

## Why

Helper-pod-based local provisioners (e.g. local-path-provisioner) pay a full pod schedule/start/observe/delete round trip per volume, serialized through one controller — under bursty churn (CI runners), volume creation queues for minutes. A node-resident CSI plugin does the same work as one `mkdir` and one bind mount.

## AI authorship

This codebase is written by AI agents working under human direction and review. This CSI driver is used in production in a self-hosted cluster, but evaluate it as you would any young project and treat production use as your own risk assessment.

## License

Apache-2.0
