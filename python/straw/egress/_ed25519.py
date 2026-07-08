"""Pure-Python Ed25519 signing (RFC 8032), used because no protobuf-adjacent
crypto dependency has been approved for this SDK. Registration signing is a
rare, latency-insensitive operation, so pure-Python performance is
acceptable. Ported from the public-domain reference implementation at
https://ed25519.cr.yp.to/python/ed25519.py.
"""

from __future__ import annotations

import hashlib

_B = 256
_Q = 2**255 - 19
_L = 2**252 + 27742317777372353535851937790883648493


def _h(m: bytes) -> bytes:
    return hashlib.sha512(m).digest()


def _expmod(b: int, e: int, m: int) -> int:
    if e == 0:
        return 1
    t = _expmod(b, e // 2, m) ** 2 % m
    if e & 1:
        t = (t * b) % m
    return t


def _inv(x: int) -> int:
    return _expmod(x, _Q - 2, _Q)


_D = -121665 * _inv(121666) % _Q
_I = _expmod(2, (_Q - 1) // 4, _Q)


def _xrecover(y: int) -> int:
    xx = (y * y - 1) * _inv(_D * y * y + 1)
    x = _expmod(xx, (_Q + 3) // 8, _Q)
    if (x * x - xx) % _Q != 0:
        x = (x * _I) % _Q
    if x % 2 != 0:
        x = _Q - x
    return x


_BY = 4 * _inv(5)
_BX = _xrecover(_BY)
_BASE = (_BX % _Q, _BY % _Q)


def _edwards(p: tuple, q: tuple) -> tuple:
    x1, y1 = p
    x2, y2 = q
    x3 = (x1 * y2 + x2 * y1) * _inv(1 + _D * x1 * x2 * y1 * y2)
    y3 = (y1 * y2 + x1 * x2) * _inv(1 - _D * x1 * x2 * y1 * y2)
    return (x3 % _Q, y3 % _Q)


def _scalarmult(p: tuple, e: int) -> tuple:
    if e == 0:
        return (0, 1)
    q = _scalarmult(p, e // 2)
    q = _edwards(q, q)
    if e & 1:
        q = _edwards(q, p)
    return q


def _encodeint(y: int) -> bytes:
    bits = [(y >> i) & 1 for i in range(_B)]
    return bytes(sum(bits[i * 8 + j] << j for j in range(8)) for i in range(_B // 8))


def _encodepoint(p: tuple) -> bytes:
    x, y = p
    bits = [(y >> i) & 1 for i in range(_B - 1)] + [x & 1]
    return bytes(sum(bits[i * 8 + j] << j for j in range(8)) for i in range(_B // 8))


def _bit(h: bytes, i: int) -> int:
    return (h[i // 8] >> (i % 8)) & 1


def _clamped_scalar(seed_hash: bytes) -> int:
    return 2 ** (_B - 2) + sum(2**i * _bit(seed_hash, i) for i in range(3, _B - 2))


def _hint(m: bytes) -> int:
    h = _h(m)
    return sum(2**i * _bit(h, i) for i in range(2 * _B))


def public_key(seed: bytes) -> bytes:
    """Derives the 32-byte Ed25519 public key for a 32-byte seed."""
    if len(seed) != 32:
        raise ValueError("seed must be 32 bytes")
    a = _clamped_scalar(_h(seed))
    return _encodepoint(_scalarmult(_BASE, a))


def sign(seed: bytes, message: bytes) -> bytes:
    """Signs message with the Ed25519 private key derived from seed."""
    if len(seed) != 32:
        raise ValueError("seed must be 32 bytes")
    h = _h(seed)
    a = _clamped_scalar(h)
    pk = _encodepoint(_scalarmult(_BASE, a))
    r = _hint(h[_B // 8 : _B // 4] + message)
    rr = _scalarmult(_BASE, r)
    s = (r + _hint(_encodepoint(rr) + pk + message) * a) % _L
    return _encodepoint(rr) + _encodeint(s)


def _isoncurve(p: tuple) -> bool:
    x, y = p
    return (-x * x + y * y - 1 - _D * x * x * y * y) % _Q == 0


def _decodeint(s: bytes) -> int:
    return sum(2**i * _bit(s, i) for i in range(_B))


def _decodepoint(s: bytes) -> tuple:
    y = sum(2**i * _bit(s, i) for i in range(_B - 1))
    x = _xrecover(y)
    if x & 1 != _bit(s, _B - 1):
        x = _Q - x
    p = (x, y)
    if not _isoncurve(p):
        raise ValueError("decoding point that is not on curve")
    return p


def verify(pub: bytes, message: bytes, signature: bytes) -> bool:
    """Verifies an Ed25519 signature. Used only by tests in this SDK."""
    if len(signature) != _B // 4 or len(pub) != _B // 8:
        return False
    try:
        r = _decodepoint(signature[: _B // 8])
        a = _decodepoint(pub)
        s = _decodeint(signature[_B // 8 : _B // 4])
        h = _hint(_encodepoint(r) + pub + message)
        x1, y1 = _scalarmult(_BASE, s)
        x2, y2 = _edwards(r, _scalarmult(a, h))
    except ValueError:
        return False
    return x1 == x2 and y1 == y2
