#!/usr/bin/env bash
set -euo pipefail

helm_command="${HELM:-helm}"
work_dir="$(mktemp -d)"

cleanup() {
  rm -rf "${work_dir}"
}
trap cleanup EXIT

"${helm_command}" template csi-driver-dirpath charts/csi-driver-dirpath \
  --namespace kube-system \
  --values hack/deploy-values.yaml >"${work_dir}/csi-driver-dirpath.yaml"
"${helm_command}" template csi-driver-dirpath charts/csi-driver-dirpath \
  --namespace kube-system \
  --values hack/kind-values.yaml >"${work_dir}/kind.yaml"

mv "${work_dir}/csi-driver-dirpath.yaml" deploy/csi-driver-dirpath.yaml
mv "${work_dir}/kind.yaml" deploy/kind.yaml
