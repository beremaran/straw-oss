
curl -s -H "Authorization: Bearer sk_live_fLcIdapXx562xATYWSBydU8d0iyHwHyGib9PEcZGtW4" \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://beremaran.com","timeout_ms":15000}' \
  http://localhost:8080/api/v1/requests | jq

curl -s http://localhost:8123/ --data-binary 'SELECT * FROM straw.request_events FORMAT Vertical'
