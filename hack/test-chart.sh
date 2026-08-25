#!/usr/bin/env bash
set -euo pipefail

helm_command="${HELM:-helm}"
rendered="$(mktemp)"

cleanup() {
  rm -f "${rendered}"
}
trap cleanup EXIT

"${helm_command}" template csi-driver-dirpath charts/csi-driver-dirpath >"${rendered}"

awk '
  $0 == "        - name: dirpath-plugin" { plugin = 1; next }
  plugin && $0 == "        - name: node-driver-registrar" { exit }
  plugin && $0 == "          command:" {
    getline
    if ($0 == "            - /csi-driver-dirpath") found = 1
  }
  END { exit !found }
' "${rendered}"
