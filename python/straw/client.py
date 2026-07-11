"""Standard-library Python client for Straw's REST request endpoint."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional
from urllib.error import HTTPError
from urllib.request import Request as _URLRequest
from urllib.request import urlopen

REQUESTS_PATH = "/api/v1/requests"
_REPLAYABLE_METHODS = {"GET", "HEAD", "OPTIONS"}


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
    mode: str = "inline_base64"
    data_base64: str = ""

    def to_json(self) -> Dict[str, str]:
        return {"mode": self.mode, "data_base64": self.data_base64}


@dataclass
class Request:
    method: str
    url: str
    headers: List[Header] = field(default_factory=list)
    body: Optional[RequestBody] = None
    fingerprint_profile: str = ""
    timeout_ms: int = 0
    replayable: bool = False

    def apply_replayable_default(self) -> None:
        if self.method.upper() in _REPLAYABLE_METHODS:
            self.replayable = True

    def to_json(self) -> Dict[str, Any]:
        data: Dict[str, Any] = {"method": self.method, "url": self.url, "replayable": self.replayable}
        if self.headers:
            data["headers"] = [header.to_json() for header in self.headers]
        if self.body is not None:
            data["body"] = self.body.to_json()
        if self.fingerprint_profile:
            data["fingerprint_profile"] = self.fingerprint_profile
        if self.timeout_ms:
            data["timeout_ms"] = self.timeout_ms
        return data


@dataclass
class ResponseBody:
    mode: str = ""
    data_base64: str = ""
    truncated: bool = False

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "ResponseBody":
        return ResponseBody(**{key: data.get(key, default) for key, default in {
            "mode": "", "data_base64": "", "truncated": False
        }.items()})


@dataclass
class Timing:
    routing_ms: int = 0
    assignment_ms: int = 0
    egress_ms: int = 0
    total_ms: int = 0

    @staticmethod
    def from_json(data: Dict[str, Any]) -> "Timing":
        return Timing(**{name: data.get(name, 0) for name in (
            "routing_ms", "assignment_ms", "egress_ms", "total_ms"
        )})


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
            headers=[Header.from_json(header) for header in data.get("headers", [])],
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
            details=data.get("details", {}),
        )


class APIError(Exception):
    def __init__(self, http_status: int, response: ErrorResponse):
        self.http_status = http_status
        self.response = response
        super().__init__(response.message or response.code)


class Client:
    def __init__(self, base_url: str, token: str = "", timeout: Optional[float] = None):
        self._base_url = base_url.rstrip("/")
        self._token = token
        self._timeout = timeout

    def do(self, request: Request) -> Response:
        request.apply_replayable_default()
        headers = {"Content-Type": "application/json"}
        if self._token:
            headers["Authorization"] = f"Bearer {self._token}"
        url_request = _URLRequest(
            self._base_url + REQUESTS_PATH,
            data=json.dumps(request.to_json()).encode("utf-8"),
            headers=headers,
            method="POST",
        )
        try:
            with urlopen(url_request, timeout=self._timeout) as response:
                raw = response.read()
        except HTTPError as exc:
            raise APIError(exc.code, ErrorResponse.from_json(json.loads(exc.read()))) from exc

        return Response.from_json(json.loads(raw))
