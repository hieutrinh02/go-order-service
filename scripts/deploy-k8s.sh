#!/usr/bin/env bash

set -euo pipefail

readonly NAMESPACE="order-service"
readonly BASE_IMAGE="ghcr.io/hieutrinh02/go-order-service:latest"
readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly EC2_OVERLAY="${PROJECT_ROOT}/deploy/k8s/overlays/ec2"
readonly BOOTSTRAP_OVERLAY="${PROJECT_ROOT}/deploy/k8s/overlays/ec2-bootstrap"
readonly MIGRATION_BASE="${PROJECT_ROOT}/deploy/k8s/base/migrate"
readonly NAMESPACE_FILE="${PROJECT_ROOT}/deploy/k8s/base/namespace.yaml"

deploy_image="${1:-}"

if [[ -z "${deploy_image}" ]]; then
  echo "usage: $0 ghcr.io/hieutrinh02/go-order-service:<immutable-tag>" >&2
  exit 1
fi

if [[ ! "${deploy_image}" =~ ^ghcr\.io/hieutrinh02/go-order-service:[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "invalid deployment image: ${deploy_image}" >&2
  exit 1
fi

if [[ "${deploy_image}" == "${BASE_IMAGE}" ]]; then
  echo "latest is not allowed; use an immutable image tag" >&2
  exit 1
fi

command -v kubectl >/dev/null 2>&1 || {
  echo "kubectl is required" >&2
  exit 1
}

render() {
  local source="$1"

  kubectl kustomize "${source}" |
    sed "s|${BASE_IMAGE}|${deploy_image}|g"
}

apply_rendered() {
  local source="$1"

  render "${source}" | kubectl apply -f -
}

wait_for_statefulsets() {
  kubectl rollout status statefulset/postgres \
    --namespace "${NAMESPACE}" \
    --timeout 5m

  kubectl rollout status statefulset/redis \
    --namespace "${NAMESPACE}" \
    --timeout 5m

  kubectl rollout status statefulset/kafka \
    --namespace "${NAMESPACE}" \
    --timeout 10m
}

wait_for_jobs() {
  kubectl wait \
    --namespace "${NAMESPACE}" \
    --for condition=complete \
    job/kafka-topic-init \
    --timeout 10m

  kubectl wait \
    --namespace "${NAMESPACE}" \
    --for condition=complete \
    job/migrate \
    --timeout 10m
}

wait_for_workloads() {
  for deployment in api publisher consumer prometheus grafana frontend; do
    kubectl rollout status "deployment/${deployment}" \
      --namespace "${NAMESPACE}" \
      --timeout 10m
  done
}

kubectl apply -f "${NAMESPACE_FILE}"

for secret in order-service-secret order-service-tls; do
  if ! kubectl get secret "${secret}" \
    --namespace "${NAMESPACE}" \
    >/dev/null 2>&1; then
    echo "required secret is missing: ${secret}" >&2
    exit 1
  fi
done

kubectl wait \
  --for condition=Established \
  crd/middlewares.traefik.io \
  --timeout 2m

if kubectl get deployment api \
  --namespace "${NAMESPACE}" \
  >/dev/null 2>&1; then
  echo "existing installation detected; running migration before rollout"

  kubectl delete job migrate \
    --namespace "${NAMESPACE}" \
    --ignore-not-found

  render "${MIGRATION_BASE}" |
    kubectl apply --namespace "${NAMESPACE}" -f -

  kubectl wait \
    --namespace "${NAMESPACE}" \
    --for condition=complete \
    job/migrate \
    --timeout 10m
else
  echo "new installation detected; running staged bootstrap"

  kubectl delete jobs kafka-topic-init migrate \
    --namespace "${NAMESPACE}" \
    --ignore-not-found

  apply_rendered "${BOOTSTRAP_OVERLAY}"
  wait_for_statefulsets
  wait_for_jobs
fi

apply_rendered "${EC2_OVERLAY}"
wait_for_workloads

kubectl get pods,services,ingresses,jobs \
  --namespace "${NAMESPACE}"

echo "Kubernetes deployment completed: ${deploy_image}"
