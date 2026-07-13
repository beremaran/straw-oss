# ADR 0003: ordered headers and bounded protocol streams

Status: accepted. Date: 2026-07-13. Owner: protocol maintainer.

Headers remain ordered pairs so duplicates and order survive the proxy boundary. Request/response bodies, frames,
credit, deadlines, and concurrency are bounded. Protocol source and generated Go/Python bindings live in public,
independently tagged repositories; conformance fixtures and negotiation coordinate changes. Do not replace ordered
headers with an unordered map or introduce unbounded buffering.
