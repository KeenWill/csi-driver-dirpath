# csi-driver-dirpath
Minimal CSI driver that provisions per-volume directories on an existing node mount via bind-mounts. DaemonSet node plugin, no helper pods, optional XFS project quotas.

**Status: specification stage.** See [SPEC.md](SPEC.md) for the full design — goals, architecture (distributed provisioning, no central controller), mount fence, XFS project quotas, packaging (Helm chart + ghcr OCI), and milestones.

## Why

Helper-pod-based local provisioners (e.g. local-path-provisioner) pay a full pod schedule/start/observe/delete round trip per volume, serialized through one controller — under bursty churn (CI runners), volume creation queues for minutes. A node-resident CSI plugin does the same work as one `mkdir` and one bind mount.

## License

Apache-2.0
