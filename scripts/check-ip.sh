curl -v -X 'POST' \
  'http://localhost:8080/v2/request' \
  -H 'accept: application/json' \
  -H 'Authorization: Bearer dev-test-token-12345' \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://httpbin.org/ip"}'
