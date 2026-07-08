import json
import os
import struct
import sys
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw import (  # noqa: E402
    REQUESTS_PATH,
    REQUESTS_STREAM_CONTENT_TYPE,
    REQUESTS_STREAM_PATH,
    APIError,
    Client,
    ErrorResponse,
    FRAME_BODY,
    FRAME_END,
    FRAME_ERROR,
    FRAME_METADATA,
    FRAME_TRAILERS,
    Header,
    Request,
)


def _write_frame(wfile, frame_type: int, payload: bytes) -> None:
    wfile.write(bytes([frame_type]) + struct.pack(">I", len(payload)) + payload)
    wfile.flush()


class _Server:
    """Runs a BaseHTTPRequestHandler-backed server in a background thread."""

    def __init__(self, handler_fn):
        class _Handler(BaseHTTPRequestHandler):
            def do_POST(self):  # noqa: N802
                handler_fn(self)

            def log_message(self, *_args):
                pass

        self._httpd = HTTPServer(("127.0.0.1", 0), _Handler)
        self._thread = threading.Thread(target=self._httpd.serve_forever, daemon=True)
        self._thread.start()

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self._httpd.server_port}"

    def close(self) -> None:
        self._httpd.shutdown()
        self._httpd.server_close()
        self._thread.join(timeout=5)


class ClientDoTests(unittest.TestCase):
    def test_encodes_request_and_defaults_replayable(self):
        captured = {}

        def handler(h):
            captured["auth"] = h.headers.get("Authorization")
            captured["path"] = h.path
            length = int(h.headers.get("Content-Length", 0))
            captured["body"] = json.loads(h.rfile.read(length))

            response = json.dumps(
                {
                    "request_id": "req_1",
                    "status": 202,
                    "body": {"mode": "inline_base64"},
                    "timing": {},
                }
            ).encode("utf-8")
            h.send_response(200)
            h.send_header("Content-Type", "application/json")
            h.end_headers()
            h.wfile.write(response)

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url, "key_1")
        resp = client.do(
            Request(
                method="GET",
                url="https://example.com/path",
                headers=[Header(name="X-Test", value_base64="dmFsdWU=")],
            )
        )

        self.assertEqual(captured["path"], REQUESTS_PATH)
        self.assertEqual(captured["auth"], "Bearer key_1")
        self.assertTrue(captured["body"]["replayable"])
        self.assertEqual(resp.status, 202)
        self.assertEqual(resp.request_id, "req_1")

    def test_replayable_defaults_only_safe_methods(self):
        for method in ("GET", "HEAD", "OPTIONS"):
            req = Request(method=method, url="https://example.com")
            req.apply_replayable_default()
            self.assertTrue(req.replayable, msg=method)

        for method in ("POST", "PUT", "PATCH", "DELETE"):
            req = Request(method=method, url="https://example.com")
            req.apply_replayable_default()
            self.assertFalse(req.replayable, msg=method)

    def test_parses_canonical_error_response(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            payload = json.dumps(
                {
                    "category": "client",
                    "code": "rate_limit_exceeded",
                    "message": "Rate limit exceeded",
                    "retryable": True,
                    "request_id": "req_2",
                    "retry_after_ms": 1500,
                    "details": {"reason": "too_many"},
                }
            ).encode("utf-8")
            h.send_response(429)
            h.send_header("Content-Type", "application/json")
            h.end_headers()
            h.wfile.write(payload)

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)

        with self.assertRaises(APIError) as ctx:
            client.do(Request(method="POST", url="https://example.com"))

        err = ctx.exception
        self.assertEqual(err.http_status, 429)
        self.assertEqual(err.response.category, "client")
        self.assertEqual(err.response.code, "rate_limit_exceeded")
        self.assertTrue(err.response.retryable)
        self.assertEqual(err.response.request_id, "req_2")
        self.assertEqual(err.response.retry_after_ms, 1500)
        self.assertEqual(err.response.details["reason"], "too_many")

    def test_treats_origin_status_as_success_envelope(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            payload = json.dumps(
                {
                    "request_id": "req_origin",
                    "status": 404,
                    "body": {"mode": "inline_base64"},
                    "timing": {},
                }
            ).encode("utf-8")
            h.send_response(200)
            h.send_header("Content-Type", "application/json")
            h.end_headers()
            h.wfile.write(payload)

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)
        resp = client.do(Request(method="POST", url="https://example.com/missing"))

        self.assertEqual(resp.status, 404)


class ClientStreamTests(unittest.TestCase):
    def test_parses_documented_frames(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)
            self.assertEqual(h.path, REQUESTS_STREAM_PATH)
            self.assertEqual(h.headers.get("Accept"), REQUESTS_STREAM_CONTENT_TYPE)

            h.send_response(200)
            h.send_header("Content-Type", REQUESTS_STREAM_CONTENT_TYPE)
            h.end_headers()

            _write_frame(
                h.wfile,
                FRAME_METADATA,
                json.dumps({"request_id": "req_stream", "status": 202}).encode("utf-8"),
            )
            _write_frame(h.wfile, FRAME_BODY, b"hel")
            _write_frame(h.wfile, FRAME_BODY, b"lo")
            _write_frame(
                h.wfile,
                FRAME_TRAILERS,
                json.dumps({"headers": [{"name": "X-Trailer", "value_base64": "ZG9uZQ=="}]}).encode("utf-8"),
            )
            _write_frame(h.wfile, FRAME_END, json.dumps({"timing": {"total_ms": 17}}).encode("utf-8"))

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)
        stream = client.do_stream(Request(method="GET", url="https://example.com"))
        self.addCleanup(stream.close)

        frame = next(stream)
        self.assertEqual(frame.type, FRAME_METADATA)
        self.assertEqual(frame.metadata.request_id, "req_stream")

        frame = next(stream)
        self.assertEqual(frame.body, b"hel")

        frame = next(stream)
        self.assertEqual(frame.body, b"lo")

        frame = next(stream)
        self.assertEqual(frame.trailers.headers[0].name, "X-Trailer")

        frame = next(stream)
        self.assertEqual(frame.end.timing.total_ms, 17)

        with self.assertRaises(StopIteration):
            next(stream)

    def test_surfaces_pre_metadata_http_error(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            payload = json.dumps({"category": "auth", "code": "auth_failure", "message": "bad key"}).encode("utf-8")
            h.send_response(401)
            h.send_header("Content-Type", "application/json")
            h.end_headers()
            h.wfile.write(payload)

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)

        with self.assertRaises(APIError) as ctx:
            client.do_stream(Request(method="POST", url="https://example.com"))

        self.assertEqual(ctx.exception.response.code, "auth_failure")

    def test_surfaces_post_metadata_error_frame(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            h.send_response(200)
            h.send_header("Content-Type", REQUESTS_STREAM_CONTENT_TYPE)
            h.end_headers()

            _write_frame(
                h.wfile,
                FRAME_METADATA,
                json.dumps({"request_id": "req_stream", "status": 200}).encode("utf-8"),
            )
            _write_frame(
                h.wfile,
                FRAME_ERROR,
                json.dumps({"category": "egress", "code": "upstream_reset", "message": "reset"}).encode("utf-8"),
            )

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)
        stream = client.do_stream(Request(method="POST", url="https://example.com"))
        self.addCleanup(stream.close)

        next(stream)
        frame = next(stream)
        self.assertEqual(frame.error.code, "upstream_reset")

    def test_malformed_frame_length_raises(self):
        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            h.send_response(200)
            h.send_header("Content-Type", REQUESTS_STREAM_CONTENT_TYPE)
            h.end_headers()
            # Declares a 5-byte payload but only sends 1 byte before closing.
            h.wfile.write(bytes([FRAME_BODY]) + struct.pack(">I", 5) + b"x")
            h.wfile.flush()

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)
        stream = client.do_stream(Request(method="GET", url="https://example.com"))
        self.addCleanup(stream.close)

        with self.assertRaises(EOFError):
            next(stream)

    def test_stream_yields_frames_incrementally_without_full_buffering(self):
        arrival_gap = 0.3

        def handler(h):
            length = int(h.headers.get("Content-Length", 0))
            h.rfile.read(length)

            h.send_response(200)
            h.send_header("Content-Type", REQUESTS_STREAM_CONTENT_TYPE)
            h.end_headers()

            _write_frame(
                h.wfile,
                FRAME_METADATA,
                json.dumps({"request_id": "req_stream", "status": 200}).encode("utf-8"),
            )
            _write_frame(h.wfile, FRAME_BODY, b"first")
            time.sleep(arrival_gap)
            _write_frame(h.wfile, FRAME_BODY, b"second")
            _write_frame(h.wfile, FRAME_END, json.dumps({"timing": {}}).encode("utf-8"))

        server = _Server(handler)
        self.addCleanup(server.close)

        client = Client(server.url)
        stream = client.do_stream(Request(method="GET", url="https://example.com"))
        self.addCleanup(stream.close)

        next(stream)  # metadata
        t0 = time.monotonic()
        first = next(stream)
        first_elapsed = time.monotonic() - t0
        second = next(stream)
        second_elapsed = time.monotonic() - t0

        self.assertEqual(first.body, b"first")
        self.assertEqual(second.body, b"second")
        # The first body frame must be observable well before the server
        # sends the second one, proving frames are consumed one at a time
        # rather than the whole response being buffered up front.
        self.assertLess(first_elapsed, arrival_gap)
        self.assertGreaterEqual(second_elapsed, arrival_gap)


class NoInternalReferenceTests(unittest.TestCase):
    def test_client_module_has_no_internal_import(self):
        client_path = os.path.join(os.path.dirname(__file__), "..", "straw", "client.py")
        with open(client_path, "r", encoding="utf-8") as fh:
            contents = fh.read()

        self.assertNotIn("internal", contents)


if __name__ == "__main__":
    unittest.main()
