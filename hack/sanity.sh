#!/usr/bin/env bash
set -euo pipefail

work_dir="$(mktemp -d)"
socket="${work_dir}/csi.sock"

cleanup() {
  if [[ -n "${driver_pid:-}" ]]; then
    kill "${driver_pid}" 2>/dev/null || true
    wait "${driver_pid}" 2>/dev/null || true
  fi
  rm -rf "${work_dir}"
}
trap cleanup EXIT

go build -o "${work_dir}/dirpath-plugin" ./cmd/dirpath-plugin
"${work_dir}/dirpath-plugin" --endpoint="unix://${socket}" --node-id=sanity-node &
driver_pid=$!

for _ in {1..50}; do
  [[ -S "${socket}" ]] && break
  sleep 0.1
done
[[ -S "${socket}" ]]

go run github.com/kubernetes-csi/csi-test/v5/cmd/csi-sanity@v5.3.1 \
  --csi.endpoint="unix://${socket}" \
  --csi.mountdir="${work_dir}/mount" \
  --csi.stagingdir="${work_dir}/staging"
