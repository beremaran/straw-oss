# P0 Testing Matrix Audit

This maps every row of `docs/planning/30-testing-matrix.md` to the tests that cover it, per task
`docs/tasks/p0/25-p0-test-matrix-and-compose.md` acceptance criterion 1. Run all with `go test ./...`.
Rows that are tool-gated or not applicable to the P0 functional suite are marked and justified.

| Matrix area | Representative tests | Notes |
|-------------|----------------------|-------|
| Protobuf | `TestStreamFrameBodyRefCompiles`, `TestAssignRequestCreditFieldsExist`, `TestValidateRejectsUnknownEnums` (api/proto/straw/v1) | `buf lint` / `buf breaking` are tool-gated: run via the `buf` CLI against `buf.yaml`/`buf.gen.yaml`; not part of `go test`. |
| NATS subjects | `TestSubjects`, `TestValidateSubjectToken`, `TestValidateMaxPayload`, `TestValidateServers`, `TestConnectAndVerifyMaxPayload` (natsx) | Exact assignment subject + safe token + max-payload validation. No pool queue-group dispatch: assignment subject is per worker/session. |
| Registration | `TestRegisterValid`, `TestRegisterRejections`, `TestRegisterInvalidCredentialKey`, `TestRegisterCapabilityOutOfScope`, `TestRegisterRevokedCredential`, `TestRegisterDuplicateSessionReplacement` | Invalid signature = `TestRegisterInvalidCredentialKey`. |
| Heartbeat | `TestHeartbeatHealthStates`, `TestHeartbeatUnavailableThenDead`, `TestHeartbeatStaleSessionIgnored` | 15s unavailable / 30s dead thresholds. |
| Worker state | `TestGlobalDisablePrecedence`, `TestTenantDisableIsolation`, `TestDrainingExclusion`, `TestCooldownExcludesThenRecovers`, `TestCooldownWindowExpiry`, `TestRegisterDuplicateSessionReplacement` | |
| Routing | `TestRoutingPriorityOrder`, `TestRoutingHardClientHints`, `TestRoutingTenantIsolation`, `TestRoutingDegradedPoolPolicy`, `TestRoutingNoMatch`, `TestRoutingStickySuccess`, `TestRoutingStickyFailure`, `TestRoutingStickyFallback` | |
| Assignment | `TestAssignmentLifecycle`, `TestWorkerRejectsAssignmentAtCapacity`, `TestDrainingExclusion`, `TestDispatcherAssignmentTimeout`, `TestAssignmentPreStartFailuresAllowFallback` | Ack timeout = `TestDispatcherAssignmentTimeout`; no-duplicate-retry = fallback boundary tests. |
| Streaming | `TestStreamValidatorRules`, `TestStreamValidatorCreditOffsetAndIdle`, `TestDispatcherStreamProtocolError`, `TestWorkerCreditExhaustionAbortsWithoutPublishing` | Sequence gap = `TestDispatcherStreamProtocolError`. |
| Terminal | `TestAssignmentLifecycle`, `TestAssignmentFallbackBoundaryAndAdminCancel`, `TestHeartbeatUnavailableThenDead` | Worker-death path: registry marks `RuntimeDead` (`worker_registry.go`), surfaced as a terminal dispatch error; exercised via the dead-state and assignment-timeout tests. |
| Cancellation | `TestDispatcherCancellation`, `TestAssignmentFallbackBoundaryAndAdminCancel`, `TestWorkerCancelFrameDuringExecutionProducesCancelledFrame`, `TestExecutorEnforcesTotalDeadline` | Client disconnect = `TestDispatcherCancellation`; admin cancel + foreign-tenant reject = `AuthorizeAdminCancel` in `TestAssignmentFallbackBoundaryAndAdminCancel`. |
| Fallback | `TestAssignmentPreStartFailuresAllowFallback`, `TestAssignmentFallbackBoundaryAndAdminCancel` | No fallback after `RequestStart` unless replayable. |
| Error mapping | `TestErrorRegistryComplete`, `TestErrorRegistryCoversEveryProtoCode`, `TestOriginStatusPassthroughIsNotErrorResponse`, `TestValidateExecutorErrorMapsOutOfSetCodes`, `TestExecutorEmittableSetMatchesContract` | Out-of-set ErrorFrame -> `executor_internal_error` = `TestValidateExecutorErrorMapsOutOfSetCodes`. |
| REST schema | `TestHandlerValidRequest`, `TestValidateRequestEmptyHostRejected`, `TestHandlerHostHeaderRejected`, `TestHandlerDuplicateHeaders`, `TestHandlerBodyLimitExceeded`, `TestHandlerCONNECTRejected` | |
| REST outcome | `TestOriginStatusPassthroughIsNotErrorResponse`, `TestHandlerSuccessEnvelopeStructure`, `TestDispatcherControlNATSEgressRoundTrip` | Upstream status returned in envelope proven end-to-end by the round-trip test (upstream 418 -> API 200). |
| Body limits | `TestHandlerBodyLimitExceeded` (request), `TestDispatcherResponseBodyTooLarge` (response, with direction) | |
| P0 exclusions | `TestValidateRequestCaptureHintOtherThanNone`, `TestHandlerCaptureHintRejected`, `TestHandlerUnknownFieldsRejected`, `TestValidateRequestBodyRefRejected`, `TestStreamValidatorRejectsP0BodyRef` | |
| Rate limits | `TestRateLimiterDimensionsAreIndependent`, `TestRateLimiterDeniesOverLimitWithRetryAfter`, `TestRateLimiterRedisFailurePolicy`, `TestRateLimiterMemoryGuardrailFallback`, `TestRateLimitCeilingRejectsExceedingValues` | |
| Quotas | `TestQuotaAdmissionRequestCount`, `TestQuotaAdmissionBandwidthAccounting`, `TestQuotaAdmissionRedisFailurePolicy`, `TestQuotaAdmissionNotBillingGrade`, `TestQuotaKeysHaveTTL` | |
| Deny rules | `TestResolveDestinationPolicy_HostDenyNormalization`, `TestResolveDestinationPolicy_CIDRAllowOverridesPrivateDefault`, `TestResolveDestinationPolicy_PrivateRangeDefaultDenied`, `TestResolveDestinationPolicy_MetadataIPDefaultDenied`, `TestExecutorBlocksDNSRebindingByDialingValidatedIP`, `TestResolveDestinationPolicy_NonASCIIHostRejected` | Redirect-target deny is a P1 future test (no redirect following in P0: `TestExecutorDoesNotFollowRedirects`). |
| Egress policy | `TestResolveDestinationPolicy_*` (RequestStart carries DestinationPolicy), `TestExecutorRejectsResolvedDeniedIPAndRedactsDetails` | Egress enforces resolved-IP deny without querying Control DBs. |
| HTTP behavior | `TestP0TransportDefaults`, `TestExecutorDoesNotFollowRedirects`, `TestHandlerCONNECTRejected` | CONNECT / HTTP-2 / keep-alive disabled in P0. |
| Redaction | `TestHandlerURLUserInfoRejected`, `TestSanitizeTargetURLDropsQuery`, `TestRedactSensitiveHeaderValue`, `TestBuildRequestEventRecordsActorAndSanitizedTarget` | |
| Config API | `TestConfigCacheSaveVersionConflict`, `TestRateLimitConfigVersionConflict`, `TestRoutingRuleCRUDAndRBAC`, `TestPostgresSaveTenantSnapshotOptimisticVersioning` | Config/admin path separation = route table in `TestRoutingRuleCRUDAndRBAC`. |
| Invalidation | `TestRedisInvalidationPublishSubscribe`, `TestConfigCacheMissedPubSubRecovery`, `TestConfigCachePollAllTenantsRecoversMissedInvalidation`, `TestConfigCacheAPIKeyRevocationInvalidation`, `TestRevokeTenantAPIKeyInvalidatesConfigCache` | |
| ClickHouse | `TestRequestMetadataWriterFlushSuccess`, `TestRequestMetadataWriterOutageKeepsQueuedEvents`, `TestRequestMetadataWriterDropsOldestWhenFull`, `TestSanitizeTargetURLDropsQuery` | Async write, outage, bounded-queue drop, sanitized target_url. Binary wiring: `wireClickHouse` in `cmd/control/main.go`. |
| Load | — | **Not applicable to the P0 functional suite.** p50/p99 routing latency and load benchmarks require a load harness; deferred to P1 observability/load work, no automated `go test` row in P0. |
| Auth | `TestPlatformAPIKeyLifecycle`, `TestPlatformKeyCannotExecuteDataPlaneRequest`, `TestTenantKeyCannotCreateTenants`, `TestAuthenticateRejectsRevokedKey`, `TestQuotaWriteRequiresPlatformKey`, `TestWorkerCredentialCreateRejectsForeignTenantScope` | |
| Audit | `TestActorAuditSourceRecorded`, `TestPostgresAuditStoreActorRecords`, `TestPostgresConfigStoreRedactsInjectionPolicyAudit`, `TestHashAPIKeySecretNeverEqualsPlaintext` | |
| Identifiers | `TestPostgresTenantStoreDuplicateRejected`, `TestWorkerCredentialCreateForcesCallerTenantScope`, `TestRegisterCapabilityOutOfScope` | Multi-tenant pool scope validated via credential tenant-scope enforcement. |
| HTTP validation | `TestValidateRequestInvalidHeaderName`, `TestValidateRequestCRInHeaderValue`, `TestHandlerURLFragmentRejected`, `TestResolveDestinationPolicy_NonASCIIHostRejected` | |
| Injection auth | `TestInjectionPolicySafetyRules`, `TestResolveDestinationPolicy_InjectionDeniedHeaderRejected` | Operator cannot set Authorization/Cookie injection; tenant_admin audited sensitive policy. |
| Worker admin | `TestTenantWorkerActionAffectsOnlyThatTenant`, `TestGlobalWorkerActionRequiresSystemAdmin`, `TestTenantWorkerListOmitsOtherTenants`, `TestListWorkersPlatformSeesSessionTenantDoesNot` | Foreign-tenant request cancel reject = `AuthorizeAdminCancel` (`TestAssignmentFallbackBoundaryAndAdminCancel`). |
| NATS ordering | `TestNATSOrderingRequiresFlushedStreamSubscription` | |
| SSRF | `TestExecutorBlocksDNSRebindingByDialingValidatedIP`, `TestExecutorDeniesPrivateAndMetadataIPsByDefault` | |
| Timeout | `TestExecutorEnforcesTotalDeadline`, `TestAckDeadlineUsesEarlierClock` | Total deadline wins over phase timeout = `TestAckDeadlineUsesEarlierClock`. |

## Outage rows (docs/planning/29)

| Outage | Test |
|--------|------|
| Redis unavailable (fail policy) | `TestRateLimiterRedisFailurePolicy`, `TestQuotaAdmissionRedisFailurePolicy`, `TestRedisStickyStoreDegradesOnRedisFailure`, `TestOpenRedisUnreachableStillReturnsClient` (Control still starts) |
| NATS unavailable (`transport_unavailable`) | `TestDispatcherNATSUnavailable` |
| ClickHouse unavailable (bounded buffer, drop oldest) | `TestRequestMetadataWriterOutageKeepsQueuedEvents`, `TestRequestMetadataWriterDropsOldestWhenFull` |
| Postgres unavailable (cached snapshots) | `TestConfigCacheSnapshotHit` (serves from cache without a store round-trip) |

## Full vertical slice

`TestDispatcherControlNATSEgressRoundTrip` drives a validated REST request through the Control dispatcher, over
NATS (assignment + request/response streams), into a live egress worker that executes against a real upstream, and
back — asserting the upstream status and body are returned in the response envelope. Liveness/readiness probes are
covered by `TestHealthzAlwaysOK` and `TestReadyzReflectsReadiness` (cmd/control).

## Not claimed

No P1/P2 rows (proxy, CONNECT tunnelling, MITM, BodyRef, payload capture, provider adapter, telemetry read APIs,
connection pooling, HTTP/2) are implemented or claimed as tested.
