# csi-driver-dirpath
Minimal CSI driver that provisions per-volume directories on an existing node mount via bind-mounts. DaemonSet node plugin, no helper pods, optional XFS project quotas.

**Status:** milestones 1 and 2 are implemented: the CSI services pass `csi-sanity`, and the unquotaed directory lifecycle, mount fence, distributed provisioning, and orphan reconciliation run end-to-end in kind. XFS project quotas and release packaging remain future milestones. See [SPEC.md](SPEC.md) for the full design.

## Why

Helper-pod-based local provisioners (e.g. local-path-provisioner) pay a full pod schedule/start/observe/delete round trip per volume, serialized through one controller — under bursty churn (CI runners), volume creation queues for minutes. A node-resident CSI plugin does the same work as one `mkdir` and one bind mount.

## AI authorship

This codebase is written by AI agents (OpenAI Codex and Anthropic Claude) working from the human-authored [SPEC.md](SPEC.md), under human direction and review. Review feedback is triaged by a human-supervised coordinator, and changes land through pull requests with automated validation (unit tests, `csi-sanity`, kind e2e). Evaluate it as you would any young storage project: read the spec, check the tests, and treat production use as your own risk assessment.

## License

Apache-2.0
