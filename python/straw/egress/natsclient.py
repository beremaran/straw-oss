"""Minimal Core NATS wire client.

Implements only what the Egress SDK needs: CONNECT handshake, PUB, SUB,
UNSUB, flush (PING/PONG round trip), and MSG delivery. No JetStream, no
clustering discovery beyond the initial INFO line, no auto-reconnect — those
are out of scope for a decoded-HTTP custom worker built on this SDK.

ponytail: single-connection, synchronous, blocking-socket client. Sufficient
for one worker process serving one assignment session; swap for an async
client (asyncio/selectors-based) if a worker needs concurrent multi-session
NATS I/O.
"""

from __future__ import annotations

import json
import socket
import time
from dataclasses import dataclass
from typing import Dict, Optional

_DEFAULT_CONNECT_TIMEOUT = 5.0


class NATSProtocolError(RuntimeError):
    """Raised when the server sends a line this client cannot parse, or
    replies with -ERR."""


@dataclass
class NATSMessage:
    subject: str
    sid: str
    reply_to: str
    data: bytes


class _LineReader:
    """Buffers bytes off a socket and yields CRLF-terminated protocol lines,
    with support for reading a fixed-size payload immediately after a line.
    """

    def __init__(self, sock: socket.socket) -> None:
        self._sock = sock
        self._buf = b""

    def read_line(self) -> bytes:
        while b"\r\n" not in self._buf:
            chunk = self._sock.recv(65536)
            if not chunk:
                raise NATSProtocolError("connection closed while reading a line")
            self._buf += chunk
        line, self._buf = self._buf.split(b"\r\n", 1)
        return line

    def read_exact(self, n: int) -> bytes:
        while len(self._buf) < n:
            chunk = self._sock.recv(65536)
            if not chunk:
                raise NATSProtocolError("connection closed while reading payload")
            self._buf += chunk
        data, self._buf = self._buf[:n], self._buf[n:]
        return data


class NATSClient:
    """A minimal, synchronous Core NATS client over one TCP connection."""

    def __init__(self, host: str, port: int, connect_timeout: float = _DEFAULT_CONNECT_TIMEOUT) -> None:
        self._sock = socket.create_connection((host, port), timeout=connect_timeout)
        self._sock.settimeout(None)
        self._reader = _LineReader(self._sock)
        self._next_sid = 1
        self.server_info: Dict = self._handshake()

    def _handshake(self) -> Dict:
        line = self._reader.read_line()
        if not line.startswith(b"INFO "):
            raise NATSProtocolError(f"expected INFO, got: {line!r}")
        info = json.loads(line[len(b"INFO ") :])
        connect_opts = {"verbose": False, "pedantic": False, "protocol": 1}
        self._write(b"CONNECT " + json.dumps(connect_opts).encode() + b"\r\n")
        return info

    def _write(self, raw: bytes) -> None:
        self._sock.sendall(raw)

    def publish(self, subject: str, payload: bytes, reply_to: str = "") -> None:
        if reply_to:
            header = f"PUB {subject} {reply_to} {len(payload)}\r\n".encode()
        else:
            header = f"PUB {subject} {len(payload)}\r\n".encode()
        self._write(header + payload + b"\r\n")

    def subscribe(self, subject: str, queue_group: str = "") -> str:
        sid = str(self._next_sid)
        self._next_sid += 1
        if queue_group:
            self._write(f"SUB {subject} {queue_group} {sid}\r\n".encode())
        else:
            self._write(f"SUB {subject} {sid}\r\n".encode())
        return sid

    def unsubscribe(self, sid: str, max_msgs: Optional[int] = None) -> None:
        if max_msgs is not None:
            self._write(f"UNSUB {sid} {max_msgs}\r\n".encode())
        else:
            self._write(f"UNSUB {sid}\r\n".encode())

    def flush(self, timeout: float = 5.0) -> None:
        """Sends PING and blocks until the matching PONG (or a -ERR) arrives,
        proving every prior PUB/SUB/UNSUB has been processed by the server —
        this is what the assignment flow's subscription-ordering rule relies
        on (docs/public/architecture.md, "Assignment Flow").
        """
        self._write(b"PING\r\n")
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            line = self._reader.read_line()
            if line == b"PONG":
                return
            if line == b"PING":
                self._write(b"PONG\r\n")
                continue
            if line.startswith(b"-ERR"):
                raise NATSProtocolError(line.decode(errors="replace"))
            if line == b"+OK":
                continue
            if line.startswith(b"MSG "):
                self._skip_msg_payload(line)
                continue
            raise NATSProtocolError(f"unexpected line during flush: {line!r}")
        raise NATSProtocolError("flush timed out waiting for PONG")

    def _skip_msg_payload(self, msg_line: bytes) -> None:
        fields = msg_line.split(b" ")
        n = int(fields[-1])
        self._reader.read_exact(n + 2)  # payload + trailing CRLF

    def next_message(self, timeout: Optional[float] = None) -> Optional[NATSMessage]:
        """Reads and returns the next MSG frame, transparently answering
        server PING and consuming +OK/PONG. Returns None on timeout.
        """
        self._sock.settimeout(timeout)
        try:
            while True:
                line = self._reader.read_line()
                if line.startswith(b"MSG "):
                    return self._read_msg(line)
                if line == b"PING":
                    self._write(b"PONG\r\n")
                    continue
                if line in (b"+OK", b"PONG"):
                    continue
                if line.startswith(b"-ERR"):
                    raise NATSProtocolError(line.decode(errors="replace"))
                raise NATSProtocolError(f"unexpected line: {line!r}")
        except socket.timeout:
            return None
        finally:
            self._sock.settimeout(None)

    def _read_msg(self, msg_line: bytes) -> NATSMessage:
        fields = msg_line.split(b" ")
        # MSG <subject> <sid> [reply-to] <#bytes>
        if len(fields) == 4:
            subject, sid, reply_to, n = fields[1], fields[2], b"", fields[3]
        elif len(fields) == 5:
            subject, sid, reply_to, n = fields[1], fields[2], fields[3], fields[4]
        else:
            raise NATSProtocolError(f"malformed MSG line: {msg_line!r}")
        payload = self._reader.read_exact(int(n))
        self._reader.read_exact(2)  # trailing CRLF after payload
        return NATSMessage(subject=subject.decode(), sid=sid.decode(), reply_to=reply_to.decode(), data=payload)

    def close(self) -> None:
        self._sock.close()
