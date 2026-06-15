#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="tokensentinel"
K8S_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Applying all TokenSentinel K8s manifests ..."
kubectl apply -k "$K8S_DIR"

echo ""
echo "==> Waiting for all pods to be ready ..."
kubectl wait --for=condition=Ready pods --all -n "$NAMESPACE" --timeout=180s

echo ""
echo "==> Seeding Redis with initial API keys ..."
kubectl wait --for=condition=Complete job/init-keys -n "$NAMESPACE" --timeout=60s

echo ""
echo "==> All services deployed in namespace '$NAMESPACE'"
kubectl get pods -n "$NAMESPACE"

echo ""
echo "==> Services:"
kubectl get svc -n "$NAMESPACE"
