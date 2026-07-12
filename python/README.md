# Straw Python SDK

The Python package contains a standard-library client for `POST /api/v1/requests` and the lower-level custom Egress
worker SDK.

```python
from straw import Client, Request

client = Client("http://localhost:8080")
response = client.do(Request(method="GET", url="https://example.com"))
print(response.status, response.request_id)
```

For bodies above the inline limit, enable object storage and use `create_receipt`, `upload_receipt_part`, and
`complete_receipt`. Pass the resulting ID with `RequestBody(mode="receipt", receipt_id=...)`; set
`response_body_mode="receipt"` for stored responses.

Pass the deployment token as the second argument outside local development:

```python
client = Client("https://straw.example.net", "your-deployment-token")
```

Run the Python tests from the repository root:

```sh
uv run --all-packages --frozen python -m unittest discover python/tests
```

See `docs/public/sdk.md` and `docs/public/egress_worker.md` for the client and custom-worker guides.
