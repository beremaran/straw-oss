import os

from straw import APIError, Client, Request

client = Client(os.getenv("STRAW_BASE_URL", "http://localhost:8080"), os.getenv("STRAW_AUTH_TOKEN", ""))
try:
    response = client.do(Request(method="GET", url="https://example.com", timeout_ms=15_000))
except APIError as error:
    raise SystemExit(f"Straw rejected request: {error}") from error
print(response.status, response.request_id)
