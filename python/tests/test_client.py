import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw import APIError, Client, Header, Request, RequestBody, ResponseBody  # noqa: E402


class _Server:
    def __init__(self, handler):
        class _Handler(BaseHTTPRequestHandler):
            def do_POST(self):  # noqa: N802
                handler(self)

            def log_message(self, *_args):
                pass

        self.httpd = HTTPServer(("127.0.0.1", 0), _Handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    @property
    def url(self):
        return f"http://127.0.0.1:{self.httpd.server_port}"

    def close(self):
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=5)


class ClientTests(unittest.TestCase):
    def test_receipt_request_and_response_types(self):
        request = Request(method="POST", url="https://example.com", body=RequestBody(
            mode="receipt", receipt_id="rcpt_1"), response_body_mode="receipt")
        self.assertEqual(request.to_json()["body"], {"mode": "receipt", "receipt_id": "rcpt_1"})
        self.assertEqual(request.to_json()["response_body_mode"], "receipt")
        body = ResponseBody.from_json({
            "mode": "receipt", "receipt_id": "rcpt_2", "size_bytes": 42, "sha256_hex": "sum"
        })
        self.assertEqual(body.receipt_id, "rcpt_2")
        self.assertEqual(body.size_bytes, 42)

    def test_request_and_response(self):
        captured = {}

        def handler(request):
            captured["auth"] = request.headers.get("Authorization")
            length = int(request.headers.get("Content-Length", 0))
            captured["body"] = json.loads(request.rfile.read(length))
            payload = json.dumps({
                "request_id": "req_1", "status": 200,
                "body": {"mode": "inline_base64", "truncated": False}, "timing": {}
            }).encode()
            request.send_response(200)
            request.send_header("Content-Type", "application/json")
            request.send_header("Content-Length", str(len(payload)))
            request.end_headers()
            request.wfile.write(payload)

        server = _Server(handler)
        self.addCleanup(server.close)
        response = Client(server.url, "secret").do(Request(
            method="GET", url="https://example.com", headers=[Header("X-Test", "dmFsdWU=")]
        ))
        self.assertEqual(captured["auth"], "Bearer secret")
        self.assertTrue(captured["body"]["replayable"])
        self.assertEqual(response.status, 200)

    def test_api_error(self):
        def handler(request):
            length = int(request.headers.get("Content-Length", 0))
            request.rfile.read(length)
            payload = json.dumps({"code": "auth_failure", "message": "Authentication failed"}).encode()
            request.send_response(401)
            request.send_header("Content-Type", "application/json")
            request.send_header("Content-Length", str(len(payload)))
            request.end_headers()
            request.wfile.write(payload)

        server = _Server(handler)
        self.addCleanup(server.close)
        with self.assertRaises(APIError) as error:
            Client(server.url, "wrong").do(Request(method="GET", url="https://example.com"))
        self.assertEqual(error.exception.response.code, "auth_failure")


if __name__ == "__main__":
    unittest.main()
