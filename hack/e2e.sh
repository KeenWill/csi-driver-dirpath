#!/usr/bin/env bash
set -euo pipefail

cluster_name="${KIND_CLUSTER_NAME:-dirpath-e2e}"
kind_command="${KIND:-kind}"
kubectl_command="${KUBECTL:-kubectl}"
node_image="${KIND_NODE_IMAGE:-kindest/node:v1.31.0@sha256:25a3504b2b340954595fa7a6ed1575ef2edadf5abd83c0776a4308b64bf47c93}"
cluster_created=false

cleanup() {
  if [[ "${cluster_created}" == "true" && "${KEEP_CLUSTER:-false}" != "true" ]]; then
    "${kind_command}" delete cluster --name "${cluster_name}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

command -v docker >/dev/null
command -v "${kind_command}" >/dev/null
command -v "${kubectl_command}" >/dev/null

if "${kind_command}" get clusters | grep -Fxq "${cluster_name}"; then
  echo "kind cluster ${cluster_name} already exists" >&2
  exit 1
fi
"${kind_command}" create cluster --name "${cluster_name}" --image "${node_image}" --config hack/kind-config.yaml
cluster_created=true
docker build -t csi-driver-dirpath:e2e .
"${kind_command}" load docker-image --name "${cluster_name}" csi-driver-dirpath:e2e

mapfile -t nodes < <("${kind_command}" get nodes --name "${cluster_name}")
for node in "${nodes[@]}"; do
  docker exec "${node}" mkdir -p /var/lib/dirpath
  docker exec "${node}" sh -c "printf %s kind-e2e-fence > /var/lib/dirpath/.dirpath-fence"
done

"${kubectl_command}" apply -f deploy/kind.yaml
"${kubectl_command}" -n kube-system rollout status daemonset/csi-dirpath --timeout=180s
"${kubectl_command}" apply -f hack/e2e-workload.yaml
"${kubectl_command}" wait --for=condition=Ready pod/dirpath-e2e --timeout=180s

node="$("${kubectl_command}" get pod dirpath-e2e -o jsonpath='{.spec.nodeName}')"
pv="$("${kubectl_command}" get pvc dirpath-e2e -o jsonpath='{.spec.volumeName}')"
handle="$("${kubectl_command}" get pv "${pv}" -o jsonpath='{.spec.csi.volumeHandle}')"

test "$("${kubectl_command}" exec dirpath-e2e -- cat /data/message)" = "dirpath-e2e"
test "$(docker exec "${node}" cat "/var/lib/dirpath/volumes/${handle}/message")" = "dirpath-e2e"

"${kubectl_command}" delete pod dirpath-e2e --wait=true
docker exec "${node}" sh -c "printf %s wrong-fence > /var/lib/dirpath/.dirpath-fence"
"${kubectl_command}" delete pvc dirpath-e2e --wait=false
sleep 8
"${kubectl_command}" get pv "${pv}" >/dev/null
docker exec "${node}" test -d "/var/lib/dirpath/volumes/${handle}"

docker exec "${node}" sh -c "printf %s kind-e2e-fence > /var/lib/dirpath/.dirpath-fence"
"${kubectl_command}" wait --for=delete "pv/${pv}" --timeout=120s
for _ in {1..60}; do
  if ! docker exec "${node}" test -e "/var/lib/dirpath/volumes/${handle}"; then
    break
  fi
  sleep 1
done
! docker exec "${node}" test -e "/var/lib/dirpath/volumes/${handle}"

orphan_node="${nodes[1]}"
docker exec "${orphan_node}" mkdir -p /var/lib/dirpath/volumes/vol-orphan-e2e
for _ in {1..30}; do
  if ! docker exec "${orphan_node}" test -e /var/lib/dirpath/volumes/vol-orphan-e2e; then
    break
  fi
  sleep 1
done
! docker exec "${orphan_node}" test -e /var/lib/dirpath/volumes/vol-orphan-e2e

echo "kind e2e passed"
