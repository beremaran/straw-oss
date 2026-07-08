"""Minimal Python client for Straw's public REST request transport.

Supported endpoints:
  - POST /api/v1/requests
  - POST /api/v1/requests:stream

``Response.status`` is the upstream HTTP status carried inside the JSON
envelope. The outer HTTP status of the call to Straw only reports whether
Straw accepted and transported the request.
"""

from __future__ import annotations

import json
import struct
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional
from urllib.error import HTTPError
from urllib.request import Request as _URLRequest
from urllib.request import urlopen

REQUESTS_PATH = "/api/v1/requests"
REQUESTS_STREAM_PATH = "/api/v1/requests:stream"
REQUESTS_STREAM_CONTENT_TYPE = "application/vnd.straw.request-stream.v1+binary"

_REPLAYABLE_DEFAULT_METHODS = {"GET", "HEAD", "OPTIONS"}

FRAME_METADATA = 1
FRAME_BODY = 2
FRAME_TRAILERS = 3
FRAME_END = 4
FRAME_ERROR = 5

_FRAME_HEADER_SIZE = 5


@dataclass
class Header:
    name: str
    value_base64: str = ""

    def to_json(self) -> Dict[str, str]:
        return {"name": self.name, "value_base64": self.value_base64}

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "Header":
        return Header(name=data["name"], value_base64=data.get("value_base64", ""))


@dataclass
class RequestBody:
    mode: str
    data_base64: str = ""

    def to_json(self) -> Dict[str, str]:
        body = {"mode": self.mode}
        if self.data_base64:
            body["data_base64"] = self.data_base64
        return body


@dataclass
class RoutingHints:
    tags: Optional[List[str]] = None
    country: str = ""
    region: str = ""
    ip_type: str = ""
    sticky_session_id: str = ""

    def to_json(self) -> Dict[str, Any]:
        hints: Dict[str, Any] = {}
        if self.tags:
            hints["tags"] = list(self.tags)
        if self.country:
            hints["country"] = self.country
        if self.region:
            hints["region"] = self.region
        if self.ip_type:
            hints["ip_type"] = self.ip_type
        if self.sticky_session_id:
            hints["sticky_session_id"] = self.sticky_session_id
        return hints


@dataclass
class Request:
    method: str
    url: str
    headers: List[Header] = field(default_factory=list)
    body: Optional[RequestBody] = None
    routing: Optional[RoutingHints] = None
    fingerprint_profile: str = ""
    timeout_ms: int = 0
    replayable: bool = False
    capture_hint: str = ""

    def apply_replayable_default(self) -> None:
        if self.method.upper() in _REPLAYABLE_DEFAULT_METHODS:
            self.replayable = True

    def to_json(self) -> Dict[str, Any]:
        envelope: Dict[str, Any] = {
            "method": self.method,
            "url": self.url,
            "replayable": self.replayable,
        }
        if self.headers:
            envelope["headers"] = [h.to_json() for h in self.headers]
        if self.body is not None:
            envelope["body"] = self.body.to_json()
        if self.routing is not None:
            routing_json = self.routing.to_json()
            if routing_json:
                envelope["routing"] = routing_json
        if self.fingerprint_profile:
            envelope["fingerprint_profile"] = self.fingerprint_profile
        if self.timeout_ms:
            envelope["timeout_ms"] = self.timeout_ms
        if self.capture_hint:
            envelope["capture_hint"] = self.capture_hint
        return envelope


@dataclass
class ResponseBody:
    mode: str = ""
    data_base64: str = ""
    truncated: bool = False

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "ResponseBody":
        return ResponseBody(
            mode=data.get("mode", ""),
            data_base64=data.get("data_base64", ""),
            truncated=data.get("truncated", False),
        )


@dataclass
class Timing:
    routing_ms: int = 0
    assignment_ms: int = 0
    egress_ms: int = 0
    total_ms: int = 0

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "Timing":
        return Timing(
            routing_ms=data.get("routing_ms", 0),
            assignment_ms=data.get("assignment_ms", 0),
            egress_ms=data.get("egress_ms", 0),
            total_ms=data.get("total_ms", 0),
        )


@dataclass
class Response:
    request_id: str
    status: int
    headers: List[Header]
    body: ResponseBody
    timing: Timing

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "Response":
        return Response(
            request_id=data.get("request_id", ""),
            status=data.get("status", 0),
            headers=[Header.from_json(h) for h in data.get("headers", [])],
            body=ResponseBody.from_json(data.get("body", {})),
            timing=Timing.from_json(data.get("timing", {})),
        )


@dataclass
class ErrorResponse:
    category: str = ""
    code: str = ""
    message: str = ""
    retryable: bool = False
    request_id: str = ""
    timeout_type: str = ""
    retry_after_ms: int = 0
    details: Dict[str, str] = field(default_factory=dict)

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "ErrorResponse":
        return ErrorResponse(
            category=data.get("category", ""),
            code=data.get("code", ""),
            message=data.get("message", ""),
            retryable=data.get("retryable", False),
            request_id=data.get("request_id", ""),
            timeout_type=data.get("timeout_type", ""),
            retry_after_ms=data.get("retry_after_ms", 0),
            details=data.get("details") or {},
        )


class APIError(Exception):
    """Raised for non-200 responses; carries the parsed canonical ErrorResponse."""

    def __init__(self, http_status: int, response: ErrorResponse):
        self.http_status = http_status
        self.response = response
        super().__init__(response.message or response.code)


@dataclass
class StreamMetadata:
    request_id: str = ""
    status: int = 0
    headers: List[Header] = field(default_factory=list)

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "StreamMetadata":
        return StreamMetadata(
            request_id=data.get("request_id", ""),
            status=data.get("status", 0),
            headers=[Header.from_json(h) for h in data.get("headers", [])],
        )


@dataclass
class StreamTrailers:
    headers: List[Header] = field(default_factory=list)

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "StreamTrailers":
        return StreamTrailers(headers=[Header.from_json(h) for h in data.get("headers", [])])


@dataclass
class StreamEnd:
    timing: Timing = field(default_factory=Timing)

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "StreamEnd":
        return StreamEnd(timing=Timing.from_json(data.get("timing", {})))


@dataclass
class StreamFrame:
    type: int
    request_id: str = ""
    metadata: Optional[StreamMetadata] = None
    body: bytes = b""
    trailers: Optional[StreamTrailers] = None
    end: Optional[StreamEnd] = None
    error: Optional[ErrorResponse] = None


def _decode_frame(frame_type: int, payload: bytes) -> StreamFrame:
    frame = StreamFrame(type=frame_type)

    if frame_type == FRAME_METADATA:
        frame.metadata = StreamMetadata.from_json(json.loads(payload))
        frame.request_id = frame.metadata.request_id
    elif frame_type == FRAME_BODY:
        frame.body = payload
    elif frame_type == FRAME_TRAILERS:
        frame.trailers = StreamTrailers.from_json(json.loads(payload))
    elif frame_type == FRAME_END:
        frame.end = StreamEnd.from_json(json.loads(payload))
    elif frame_type == FRAME_ERROR:
        frame.error = ErrorResponse.from_json(json.loads(payload))
    else:
        raise ValueError(f"unknown stream frame type {frame_type}")

    return frame


def _read_exact(fp: Any, size: int) -> Optional[bytes]:
    """Read exactly ``size`` bytes. Returns None on a clean EOF at the start
    of the read (no bytes available); raises EOFError on a truncated read.
    """
    buf = bytearray()
    while len(buf) < size:
        chunk = fp.read(size - len(buf))
        if not chunk:
            if not buf:
                return None
            raise EOFError("truncated stream frame")
        buf.extend(chunk)
    return bytes(buf)


class Stream:
    """Iterates decoded frames from POST /api/v1/requests:stream.

    Reads exactly one frame header and payload per step, so response bytes
    are never buffered ahead of what the caller has consumed.
    """

    def __init__(self, response: Any):
        self._response = response

    def close(self) -> None:
        self._response.close()

    def __enter__(self) -> "Stream":
        return self

    def __exit__(self, *_exc: Any) -> None:
        self.close()

    def __iter__(self) -> "Stream":
        return self

    def __next__(self) -> StreamFrame:
        header = _read_exact(self._response, _FRAME_HEADER_SIZE)
        if header is None:
            raise StopIteration

        frame_type = header[0]
        (length,) = struct.unpack(">I", header[1:])

        payload = _read_exact(self._response, length)
        if payload is None:
            payload = b""

        return _decode_frame(frame_type, payload)


class Client:
    """Submits requests to Straw's public API."""

    def __init__(self, base_url: str, api_key: str = "", timeout: Optional[float] = None):
        self._base_url = base_url.rstrip("/")
        self._api_key = api_key
        self._timeout = timeout

    def _headers(self, content_type: str, accept: Optional[str] = None) -> Dict[str, str]:
        headers = {"Content-Type": content_type}
        if accept:
            headers["Accept"] = accept
        if self._api_key:
            headers["Authorization"] = f"Bearer {self._api_key}"
        return headers

    def do(self, request: Request) -> Response:
        """Submits a blocking request to POST /api/v1/requests."""
        request.apply_replayable_default()

        body = json.dumps(request.to_json()).encode("utf-8")
        url_request = _URLRequest(
            self._base_url + REQUESTS_PATH,
            data=body,
            headers=self._headers("application/json"),
            method="POST",
        )

        try:
            with urlopen(url_request, timeout=self._timeout) as http_response:
                raw = http_response.read()
        except HTTPError as exc:
            raise _api_error_from_http_error(exc) from exc

        return Response.from_json(json.loads(raw))

    def do_stream(self, request: Request) -> Stream:
        """Submits a request to POST /api/v1/requests:stream and returns a frame iterator."""
        request.apply_replayable_default()

        body = json.dumps(request.to_json()).encode("utf-8")
        url_request = _URLRequest(
            self._base_url + REQUESTS_STREAM_PATH,
            data=body,
            headers=self._headers("application/json", accept=REQUESTS_STREAM_CONTENT_TYPE),
            method="POST",
        )

        try:
            http_response = urlopen(url_request, timeout=self._timeout)
        except HTTPError as exc:
            raise _api_error_from_http_error(exc) from exc

        return Stream(http_response)


def _api_error_from_http_error(exc: HTTPError) -> APIError:
    raw = exc.read()
    error_response = ErrorResponse.from_json(json.loads(raw))
    return APIError(exc.code, error_response)
