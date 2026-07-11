import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw import APIError, Client, Header, Request  # noqa: E402


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
            request.send_response(401)
            request.end_headers()
            request.wfile.write(json.dumps({"code": "auth_failure", "message": "Authentication failed"}).encode())

        server = _Server(handler)
        self.addCleanup(server.close)
        with self.assertRaises(APIError) as error:
            Client(server.url, "wrong").do(Request(method="GET", url="https://example.com"))
        self.assertEqual(error.exception.response.code, "auth_failure")


if __name__ == "__main__":
    unittest.main()
