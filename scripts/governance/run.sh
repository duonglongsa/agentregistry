#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
export PATH="/opt/homebrew/bin:$PATH"
export AGENT_REGISTRY_GOVERNANCE_ENABLED=true
export AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO=true
export AGENT_REGISTRY_GOVERNANCE_AUTH_FILE="$PWD/.local/governance/auth.json"
export AGENT_REGISTRY_SERVER_ADDRESS=127.0.0.1:12121
export AGENT_REGISTRY_DATABASE_URL='postgres://agentregistry:agentregistry@127.0.0.1:5432/agentregistry_governance_demo?sslmode=disable'
export AGENT_REGISTRY_ENABLE_REGISTRY_VALIDATION=false
if [[ ! -f "$AGENT_REGISTRY_GOVERNANCE_AUTH_FILE" ]]; then
  echo 'Run: go run ./hack/governance-demo (creates local accounts; no real credentials).'
  exit 1
fi
exec go run ./cmd/server
