#!/bin/sh
set -e

echo "Starting WireMock servers..."
docker compose -f docker-compose.test.yml up -d

echo "Waiting for mock-github to be healthy..."
until curl -sf http://localhost:8080/__admin/health > /dev/null 2>&1; do
  sleep 1
done

echo "Waiting for mock-gitlab to be healthy..."
until curl -sf http://localhost:8081/__admin/health > /dev/null 2>&1; do
  sleep 1
done

echo "Running E2E tests..."
REVIEWBRIDGE_GITHUB_BASE_URL=http://localhost:8080 \
REVIEWBRIDGE_GITLAB_BASE_URL=http://localhost:8081 \
go test ./tests/e2e/... -v -timeout 120s
EXIT_CODE=$?

echo "Tearing down WireMock servers..."
docker compose -f docker-compose.test.yml down

exit $EXIT_CODE
