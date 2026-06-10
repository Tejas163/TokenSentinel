#!/bin/sh
# Seed virtual API keys into Redis for development.
# Usage: REDIS_URL=redis://localhost:6379 sh scripts/seed-keys.sh

REDIS_URL="${REDIS_URL:-redis://localhost:6379}"

echo "Seeding virtual API keys into $REDIS_URL ..."

# Dev key with full access (compatible with older env-var-based configs)
redis-cli -u "$REDIS_URL" HSET \
  apikey:dev-key-123 \
  name "Dev Key" \
  team "engineering" \
  status "active" \
  budget_monthly_cents "500000" \
  rate_limit_rps "100" \
  allowed_models '["*"]' \
  allowed_services '["proxy","mcp","dashboard"]' \
  created_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "Done. Key 'dev-key-123' seeded with team=engineering."
