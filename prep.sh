docker compose down --volumes

STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=sk_example_admin_local docker compose up -d --build

curl -fsS http://localhost:9090/readyz && echo "Control plane ready"

curl -s -H "Authorization: Bearer sk_example_admin_local" \\n  -H 'Content-Type: application/json' \\n  -d '{"role":"requester"}' \\n  http://localhost:8080/api/v1/config/tenants/22222222-2222-4222-8222-222222222222/api-keys
