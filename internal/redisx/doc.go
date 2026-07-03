// Package redisx provides the shared Redis client helper for Straw services.
//
// Redis stores ephemeral runtime state only
// (docs/planning/21-state-and-storage.md): rate-limit counters, quota hot
// counters, sticky-session pins, and worker runtime caches. Every key
// written by Straw through a client built here must carry a TTL; Redis data
// loss must never be required to reconstruct durable config.
package redisx
