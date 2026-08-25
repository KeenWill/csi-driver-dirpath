#!/usr/bin/env bash
set -euo pipefail

helm_command="${HELM:-helm}"
rendered="$(mktemp)"
overrides="$(mktemp)"

cleanup() {
  rm -f "${rendered}" "${overrides}"
}
trap cleanup EXIT

"${helm_command}" template csi-driver-dirpath charts/csi-driver-dirpath >"${rendered}"
"${helm_command}" template csi-driver-dirpath charts/csi-driver-dirpath \
  --set driverName=invalid.example \
  --set-string 'podLabels.app\.kubernetes\.io/name=invalid-name' \
  --set-string 'podLabels.app\.kubernetes\.io/instance=invalid-instance' \
  >"${overrides}"

awk '
  $0 == "        - name: dirpath-plugin" { plugin = 1; next }
  plugin && $0 == "        - name: node-driver-registrar" { exit }
  plugin && $0 == "          command:" {
    getline
    if ($0 == "            - /dirpath-plugin") found = 1
  }
  END { exit !found }
' "${rendered}"

if grep -q 'invalid\.example\|invalid-name\|invalid-instance' "${overrides}"; then
  echo "reserved chart values changed the rendered driver identity" >&2
  exit 1
fi

grep -q '^      nodeSelector:$' "${rendered}"
grep -q '^        kubernetes.io/os: linux$' "${rendered}"
grep -q '^                - amd64$' "${rendered}"
grep -q '^                - arm64$' "${rendered}"
