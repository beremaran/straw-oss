import json
import os
import socket
import sys
import threading
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from straw.egress import NATSClient, NATSProtocolError  # noqa: E402


class _FakeNATSServer:
    """A minimal loopback Core NATS server: sends INFO, expects CONNECT,
    answers PING with PONG, and can push MSG frames or arbitrary raw lines to
    the client on demand. Enough to exercise the wire client's framing
    without a real NATS server binary.
    """

    def __init__(self):
        self._listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._listener.bind(("127.0.0.1", 0))
        self._listener.listen(1)
        self.port = self._listener.getsockname()[1]
        self.conn = None
        self.received_lines = []
        self._accept_thread = threading.Thread(target=self._accept, daemon=True)
        self._accept_thread.start()

    def _accept(self):
        conn, _ = self._listener.accept()
        self.conn = conn
        conn.sendall(b'INFO {"server_id":"fake","max_payload":1048576}\r\n')

    def wait_for_connect(self, timeout=5.0):
        self.conn.settimeout(timeout)
        buf = b""
        while b"\r\n" not in buf:
            chunk = self.conn.recv(65536)
            if not chunk:
                raise AssertionError("client closed before sending CONNECT")
            buf += chunk
        line, _ = buf.split(b"\r\n", 1)
        assert line.startswith(b"CONNECT "), line
        return json.loads(line[len(b"CONNECT ") :])

    def send_raw(self, raw: bytes) -> None:
        self.conn.sendall(raw)

    def send_msg(self, subject: str, sid: str, payload: bytes, reply_to: str = "") -> None:
        if reply_to:
            header = f"MSG {subject} {sid} {reply_to} {len(payload)}\r\n".encode()
        else:
            header = f"MSG {subject} {sid} {len(payload)}\r\n".encode()
        self.conn.sendall(header + payload + b"\r\n")

    def close(self):
        if self.conn:
            self.conn.close()
        self._listener.close()


class NATSClientTests(unittest.TestCase):
    def setUp(self):
        self.server = _FakeNATSServer()
        self.client = NATSClient("127.0.0.1", self.server.port)
        self.server.wait_for_connect()

    def tearDown(self):
        self.client.close()
        self.server.close()

    def test_publish_frames_pub_correctly(self):
        self.client.publish("straw.v1.control.register", b"hello")
        self.server.conn.settimeout(5.0)
        line = self.server.conn.recv(4096)
        self.assertEqual(line, b"PUB straw.v1.control.register 5\r\nhello\r\n")

    def test_publish_with_reply_to(self):
        self.client.publish("straw.v1.control.register", b"hi", reply_to="_INBOX.ctl.abc")
        line = self.server.conn.recv(4096)
        self.assertEqual(line, b"PUB straw.v1.control.register _INBOX.ctl.abc 2\r\nhi\r\n")

    def test_subscribe_emits_sub_line(self):
        sid = self.client.subscribe("straw.v1.req.r1.w1.s1.c2e")
        line = self.server.conn.recv(4096)
        self.assertEqual(line, f"SUB straw.v1.req.r1.w1.s1.c2e {sid}\r\n".encode())

    def test_flush_waits_for_pong(self):
        done = threading.Event()
        result = {}

        def do_flush():
            try:
                self.client.flush(timeout=5.0)
                result["ok"] = True
            except Exception as exc:  # noqa: BLE001
                result["error"] = exc
            done.set()

        t = threading.Thread(target=do_flush)
        t.start()

        line = self.server.conn.recv(4096)
        self.assertEqual(line, b"PING\r\n")
        self.server.send_raw(b"PONG\r\n")

        done.wait(timeout=5.0)
        t.join(timeout=5.0)
        self.assertTrue(result.get("ok"), result.get("error"))

    def test_next_message_parses_msg_frame(self):
        self.server.send_msg("straw.v1.req.r1.w1.s1.c2e", "42", b"\x01\x02\x03", reply_to="")
        msg = self.client.next_message(timeout=5.0)
        self.assertIsNotNone(msg)
        self.assertEqual(msg.subject, "straw.v1.req.r1.w1.s1.c2e")
        self.assertEqual(msg.sid, "42")
        self.assertEqual(msg.data, b"\x01\x02\x03")

    def test_next_message_with_reply_to(self):
        self.server.send_msg("straw.v1.control.register", "1", b"payload", reply_to="_INBOX.ctl.xyz")
        msg = self.client.next_message(timeout=5.0)
        self.assertEqual(msg.reply_to, "_INBOX.ctl.xyz")
        self.assertEqual(msg.data, b"payload")

    def test_next_message_times_out(self):
        msg = self.client.next_message(timeout=0.2)
        self.assertIsNone(msg)

    def test_flush_raises_on_err(self):
        def do_flush():
            with self.assertRaises(NATSProtocolError):
                self.client.flush(timeout=5.0)

        t = threading.Thread(target=do_flush)
        t.start()
        self.server.conn.recv(4096)  # PING
        self.server.send_raw(b"-ERR 'Unknown Protocol Operation'\r\n")
        t.join(timeout=5.0)


if __name__ == "__main__":
    unittest.main()
