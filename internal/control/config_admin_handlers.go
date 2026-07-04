package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/beremaran/straw/v2/internal/config"
)

// This file implements the P0 config-management HTTP surface for routing
// rules, deny rules, injection policies, and read-only fingerprint profiles
// (docs/tasks/p0/20, docs/planning/26-config-management-api-surface.md). Every
// handler authenticates, enforces RBAC, and — for mutating endpoints —
// increments the tenant config version and publishes invalidation through
// AdminHandlers.ConfigCache, matching the existing API-key/worker-credential
// pattern in admin_handlers.go.
//
// Optional store fields (RoutingRules, DenyRules, InjectionPolicies,
// FingerprintProfiles) are nil-checked so existing unit tests that build a
// bare AdminHandlers for other endpoints keep compiling; a handler on a nil
// store responds control_internal_error rather than panicking.

const deniedInjectionHeaderPrefix = "x-straw-"

// Deny-rule types/actions, matching the deny_rules CHECK constraints
// (migrations/postgres/0001_init.sql).
const (
	denyRuleTypeHost  = "host"
	denyRuleTypeCIDR  = "cidr"
	denyRuleTypeCName = "cname"
	denyRuleTypeIP    = "ip"

	denyRuleActionDeny  = "deny"
	denyRuleActionAllow = "allow"
)

// Injection operation verbs (docs/planning/26 Injection Policy schema).
const (
	injectionOpSet    = "set"
	injectionOpAppend = "append"
	injectionOpRemove = "remove"
)

var alwaysDeniedInjectionHeaders = map[string]bool{
	denyRuleTypeHost:      true,
	"content-length":      true,
	"transfer-encoding":   true,
	"connection":          true,
	"proxy-authorization": true,
}

var sensitiveInjectionHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
}

// ---- shared pagination ----

func parsePagination(r *http.Request) (int, int) {
	limit := defaultConfigListLimit
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			limit = n
		}
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 0 {
			offset = n
		}
	}

	return clampConfigListLimit(limit), offset
}

// ---- routing rules ----

type matchConditionsJSONBody struct {
	Tags        []string `json:"tags,omitempty"`
	Country     string   `json:"country,omitempty"`
	Region      string   `json:"region,omitempty"`
	IPType      string   `json:"ip_type,omitempty"`
	IngressType string   `json:"ingress_type,omitempty"`
	TargetHost  string   `json:"target_host,omitempty"`
}

func (m matchConditionsJSONBody) toConfig() config.MatchConditions {
	return config.MatchConditions{
		Tags: m.Tags, Country: m.Country, Region: m.Region,
		IPType: m.IPType, IngressType: m.IngressType, TargetHost: m.TargetHost,
	}
}

func matchConditionsFromConfig(m config.MatchConditions) matchConditionsJSONBody {
	return matchConditionsJSONBody{
		Tags: m.Tags, Country: m.Country, Region: m.Region,
		IPType: m.IPType, IngressType: m.IngressType, TargetHost: m.TargetHost,
	}
}

type routingRuleRequest struct {
	ID                      string                  `json:"id"`
	Priority                int                     `json:"priority"`
	Enabled                 *bool                   `json:"enabled"`
	MatchConditions         matchConditionsJSONBody `json:"match_conditions"`
	TargetPoolID            string                  `json:"target_pool_id"`
	StickySessionTTLSeconds uint32                  `json:"sticky_session_ttl_seconds"`
	AllowStickyFallback     bool                    `json:"allow_sticky_fallback"`
	ExpectedConfigVersion   uint64                  `json:"expected_config_version"`
}

type routingRuleResponse struct {
	ID                      string                  `json:"id"`
	TenantID                string                  `json:"tenant_id"`
	Priority                int                     `json:"priority"`
	Enabled                 bool                    `json:"enabled"`
	MatchConditions         matchConditionsJSONBody `json:"match_conditions"`
	TargetPoolID            string                  `json:"target_pool_id"`
	StickySessionTTLSeconds uint32                  `json:"sticky_session_ttl_seconds"`
	AllowStickyFallback     bool                    `json:"allow_sticky_fallback"`
	ConfigVersion           uint64                  `json:"config_version"`
}

func toRoutingRuleResponse(r RoutingRuleRecord) routingRuleResponse {
	return routingRuleResponse{
		ID:                      r.ID,
		TenantID:                r.TenantID,
		Priority:                r.Priority,
		Enabled:                 r.Enabled,
		MatchConditions:         matchConditionsFromConfig(r.Match),
		TargetPoolID:            r.TargetPoolID,
		StickySessionTTLSeconds: r.StickySessionTTLSeconds,
		AllowStickyFallback:     r.AllowStickyFallback,
		ConfigVersion:           r.ConfigVersion,
	}
}

func boolOrDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}

	return *v
}

// ListRoutingRules handles GET /api/v1/config/routing-rules.
func (h *AdminHandlers) ListRoutingRules(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator, RoleViewer)
	if !ok {
		return
	}

	if h.RoutingRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	limit, offset := parsePagination(r)

	records, err := h.RoutingRules.ListRoutingRules(r.Context(), identity.TenantID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]routingRuleResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toRoutingRuleResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// CreateRoutingRule handles POST /api/v1/config/routing-rules. Routing rules
// use a client-supplied stable ID (docs/planning/26).
func (h *AdminHandlers) CreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	var req routingRuleRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	if req.ID == "" || req.TargetPoolID == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
			errorDetailReasonKey: "id and target_pool_id are required",
		}))

		return
	}

	h.upsertRoutingRule(w, r, identity, req)
}

// UpdateRoutingRule handles PUT /api/v1/config/routing-rules/{id}.
func (h *AdminHandlers) UpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	var req routingRuleRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	req.ID = r.PathValue("id")
	if req.TargetPoolID == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
			errorDetailReasonKey: "target_pool_id is required",
		}))

		return
	}

	h.upsertRoutingRule(w, r, identity, req)
}

func (h *AdminHandlers) upsertRoutingRule(w http.ResponseWriter, r *http.Request, identity Identity, req routingRuleRequest) {
	if h.RoutingRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	rule := config.RoutingRule{
		ID:                      req.ID,
		Priority:                req.Priority,
		Enabled:                 boolOrDefault(req.Enabled, true),
		Match:                   req.MatchConditions.toConfig(),
		TargetPoolID:            req.TargetPoolID,
		StickySessionTTLSeconds: req.StickySessionTTLSeconds,
		AllowStickyFallback:     req.AllowStickyFallback,
	}

	record, tenantVersion, err := h.RoutingRules.UpsertRoutingRule(r.Context(), identity.TenantID, rule, req.ExpectedConfigVersion, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, func(ctx context.Context) (uint64, error) {
			got, getErr := h.RoutingRules.GetRoutingRule(ctx, identity.TenantID, req.ID)
			if getErr != nil {
				return 0, fmt.Errorf("get routing rule: %w", getErr)
			}

			return got.ConfigVersion, nil
		})

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "routing_rule", record.ID, configActionUpsert)

	writeJSON(w, http.StatusOK, toRoutingRuleResponse(record))
}

// DeleteRoutingRule handles DELETE /api/v1/config/routing-rules/{id}.
func (h *AdminHandlers) DeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	if h.RoutingRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id := r.PathValue("id")

	tenantVersion, err := h.RoutingRules.DeleteRoutingRule(r.Context(), identity.TenantID, id, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, nil)

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "routing_rule", id, configActionDelete)

	w.WriteHeader(http.StatusNoContent)
}

// ---- executor pools ----

type executorPoolRequest struct {
	ID                    string   `json:"id"`
	ExecutorType          string   `json:"executor_type"`
	Tags                  []string `json:"tags"`
	Enabled               *bool    `json:"enabled"`
	AllowDegradedWorkers  bool     `json:"allow_degraded_workers"`
	ExpectedConfigVersion uint64   `json:"expected_config_version"`
}

type executorPoolResponse struct {
	ID                   string   `json:"id"`
	TenantID             string   `json:"tenant_id"`
	ExecutorType         string   `json:"executor_type"`
	Tags                 []string `json:"tags"`
	Enabled              bool     `json:"enabled"`
	AllowDegradedWorkers bool     `json:"allow_degraded_workers"`
	ConfigVersion        uint64   `json:"config_version"`
}

func toExecutorPoolResponse(r ExecutorPoolRecord) executorPoolResponse {
	return executorPoolResponse{
		ID:                   r.ID,
		TenantID:             r.TenantID,
		ExecutorType:         defaultExecutorType(r.ExecutorType),
		Tags:                 nonNilStrings(r.Tags),
		Enabled:              r.Enabled,
		AllowDegradedWorkers: r.AllowDegradedWorkers,
		ConfigVersion:        r.ConfigVersion,
	}
}

// ListExecutorPools handles GET /api/v1/config/executor-pools.
func (h *AdminHandlers) ListExecutorPools(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator, RoleViewer)
	if !ok {
		return
	}

	if h.ExecutorPools == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	limit, offset := parsePagination(r)

	records, err := h.ExecutorPools.ListExecutorPools(r.Context(), identity.TenantID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]executorPoolResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toExecutorPoolResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// CreateExecutorPool handles POST /api/v1/config/executor-pools. Executor
// pools use a client-supplied stable ID (docs/planning/26).
func (h *AdminHandlers) CreateExecutorPool(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	var req executorPoolRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	if req.ID == "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{
			errorDetailReasonKey: "id is required",
		}))

		return
	}

	h.upsertExecutorPool(w, r, identity, req)
}

// UpdateExecutorPool handles PUT /api/v1/config/executor-pools/{id}.
func (h *AdminHandlers) UpdateExecutorPool(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	var req executorPoolRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	req.ID = r.PathValue("id")

	h.upsertExecutorPool(w, r, identity, req)
}

func (h *AdminHandlers) upsertExecutorPool(w http.ResponseWriter, r *http.Request, identity Identity, req executorPoolRequest) {
	if h.ExecutorPools == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	pool := config.ExecutorPool{
		ID:                   req.ID,
		ExecutorType:         defaultExecutorType(req.ExecutorType),
		Tags:                 req.Tags,
		Enabled:              boolOrDefault(req.Enabled, true),
		AllowDegradedWorkers: req.AllowDegradedWorkers,
	}

	record, tenantVersion, err := h.ExecutorPools.UpsertExecutorPool(r.Context(), identity.TenantID, pool, req.ExpectedConfigVersion, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, func(ctx context.Context) (uint64, error) {
			got, getErr := h.ExecutorPools.GetExecutorPool(ctx, identity.TenantID, req.ID)
			if getErr != nil {
				return 0, fmt.Errorf("get executor pool: %w", getErr)
			}

			return got.ConfigVersion, nil
		})

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "executor_pool", record.ID, configActionUpsert)

	writeJSON(w, http.StatusOK, toExecutorPoolResponse(record))
}

// DeleteExecutorPool handles DELETE /api/v1/config/executor-pools/{id}.
func (h *AdminHandlers) DeleteExecutorPool(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	if h.ExecutorPools == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id := r.PathValue("id")

	tenantVersion, err := h.ExecutorPools.DeleteExecutorPool(r.Context(), identity.TenantID, id, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, nil)

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "executor_pool", id, configActionDelete)

	w.WriteHeader(http.StatusNoContent)
}

// ---- deny rules ----

// denyRuleTypes/denyRuleActions mirror the CHECK constraints on the deny_rules
// table (migrations/postgres/0001_init.sql). docs/planning/26 documents a
// broader P1-leaning type/action taxonomy (host_suffix, cname_suffix,
// metadata_ip, private_range, allow_override); this handoff flags that gap
// rather than silently narrowing or widening it without a schema change.
var (
	denyRuleTypes   = map[string]bool{denyRuleTypeHost: true, denyRuleTypeCIDR: true, denyRuleTypeCName: true, denyRuleTypeIP: true}
	denyRuleActions = map[string]bool{denyRuleActionDeny: true, denyRuleActionAllow: true}
)

type denyRuleRequest struct {
	Enabled               *bool  `json:"enabled"`
	Type                  string `json:"type"`
	Value                 string `json:"value"`
	Action                string `json:"action"`
	ExpectedConfigVersion uint64 `json:"expected_config_version"`
}

type denyRuleResponse struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	Enabled       bool   `json:"enabled"`
	Type          string `json:"type"`
	Value         string `json:"value"`
	Action        string `json:"action"`
	ConfigVersion uint64 `json:"config_version"`
}

func toDenyRuleResponse(r DenyRuleRecord) denyRuleResponse {
	return denyRuleResponse{
		ID:            r.ID,
		TenantID:      r.TenantID,
		Enabled:       r.Enabled,
		Type:          r.RuleType,
		Value:         denyRuleValue(r.DenyRule),
		Action:        r.Action,
		ConfigVersion: r.ConfigVersion,
	}
}

func denyRuleValue(r config.DenyRule) string {
	switch r.RuleType {
	case denyRuleTypeCIDR:
		return r.NormalizedCIDR
	case denyRuleTypeIP:
		return r.NormalizedIP
	case denyRuleTypeCName:
		return r.NormalizedName
	default:
		return r.NormalizedHost
	}
}

// normalizeDenyRule validates and normalizes a deny-rule value per its type
// (docs/planning/27 "Destination Deny Normalization and CIDR Defaults":
// lowercase hostnames, trailing-dot trimming, valid CIDR/IP literals).
func normalizeDenyRule(ruleType, value string) (config.DenyRule, error) {
	if !denyRuleTypes[ruleType] {
		return config.DenyRule{}, errInvalidDenyRuleType
	}

	if value == "" {
		return config.DenyRule{}, errInvalidDenyRuleValue
	}

	rule := config.DenyRule{RuleType: ruleType, RawPattern: value}

	switch ruleType {
	case denyRuleTypeHost:
		rule.NormalizedHost = normalizeHostname(value)
	case denyRuleTypeCName:
		rule.NormalizedName = normalizeHostname(value)
	case denyRuleTypeCIDR:
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				return config.DenyRule{}, errInvalidDenyRuleValue
			}

			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}

		rule.NormalizedCIDR = prefix.String()
	case denyRuleTypeIP:
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return config.DenyRule{}, errInvalidDenyRuleValue
		}

		rule.NormalizedIP = addr.String()
	}

	return rule, nil
}

func normalizeHostname(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

var (
	errInvalidDenyRuleType  = errors.New("invalid deny rule type")
	errInvalidDenyRuleValue = errors.New("invalid deny rule value for its type")
)

// ListDenyRules handles GET /api/v1/config/deny-rules.
func (h *AdminHandlers) ListDenyRules(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator, RoleViewer)
	if !ok {
		return
	}

	if h.DenyRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	limit, offset := parsePagination(r)

	records, err := h.DenyRules.ListDenyRules(r.Context(), identity.TenantID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]denyRuleResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toDenyRuleResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// CreateDenyRule handles POST /api/v1/config/deny-rules. Deny rules are
// tenant_admin-only writes with a server-generated ID (docs/planning/26).
func (h *AdminHandlers) CreateDenyRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	var req denyRuleRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	if !denyRuleActions[req.Action] {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: "action must be deny or allow"}))

		return
	}

	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	h.upsertDenyRule(w, r, identity, id, req)
}

// UpdateDenyRule handles PUT /api/v1/config/deny-rules/{id}.
func (h *AdminHandlers) UpdateDenyRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	var req denyRuleRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	if !denyRuleActions[req.Action] {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: "action must be deny or allow"}))

		return
	}

	h.upsertDenyRule(w, r, identity, r.PathValue("id"), req)
}

func (h *AdminHandlers) upsertDenyRule(w http.ResponseWriter, r *http.Request, identity Identity, id string, req denyRuleRequest) {
	if h.DenyRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	rule, err := normalizeDenyRule(req.Type, req.Value)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: err.Error()}))

		return
	}

	rule.ID = id
	rule.Enabled = boolOrDefault(req.Enabled, true)
	rule.Action = req.Action

	record, tenantVersion, err := h.DenyRules.UpsertDenyRule(r.Context(), identity.TenantID, rule, req.ExpectedConfigVersion, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, func(ctx context.Context) (uint64, error) {
			got, getErr := h.DenyRules.GetDenyRule(ctx, identity.TenantID, id)
			if getErr != nil {
				return 0, fmt.Errorf("get deny rule: %w", getErr)
			}

			return got.ConfigVersion, nil
		})

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "deny_rule", record.ID, configActionUpsert)

	writeJSON(w, http.StatusOK, toDenyRuleResponse(record))
}

// DeleteDenyRule handles DELETE /api/v1/config/deny-rules/{id}.
func (h *AdminHandlers) DeleteDenyRule(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin)
	if !ok {
		return
	}

	if h.DenyRules == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id := r.PathValue("id")

	tenantVersion, err := h.DenyRules.DeleteDenyRule(r.Context(), identity.TenantID, id, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, nil)

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "deny_rule", id, configActionDelete)

	w.WriteHeader(http.StatusNoContent)
}

// ---- injection policies ----

type injectionOperationBody struct {
	Op          string `json:"op"`
	HeaderName  string `json:"header_name"`
	ValueBase64 string `json:"value_base64"`
}

type injectionPolicyRequest struct {
	Enabled               *bool                    `json:"enabled"`
	Operations            []injectionOperationBody `json:"operations"`
	ExpectedConfigVersion uint64                   `json:"expected_config_version"`
}

type injectionPolicyResponse struct {
	ID            string                   `json:"id"`
	TenantID      string                   `json:"tenant_id"`
	Enabled       bool                     `json:"enabled"`
	Operations    []injectionOperationBody `json:"operations"`
	MaxOperations int                      `json:"max_operations"`
	ConfigVersion uint64                   `json:"config_version"`
}

func toInjectionPolicyResponse(r InjectionPolicyRecord) injectionPolicyResponse {
	ops := make([]injectionOperationBody, 0, len(r.Operations))
	for _, op := range r.Operations {
		ops = append(ops, injectionOperationBody{Op: op.Op, HeaderName: op.HeaderName, ValueBase64: op.ValueBase64})
	}

	return injectionPolicyResponse{
		ID: r.ID, TenantID: r.TenantID, Enabled: r.Enabled, Operations: ops,
		MaxOperations: maxInjectionOperations, ConfigVersion: r.ConfigVersion,
	}
}

// validateInjectionOperations enforces the header-injection safety rules from
// docs/planning/27 ("Header Stripping", "Header Injection Safety Rules"):
// Host/Content-Length/Transfer-Encoding/Connection/Proxy-Authorization/
// X-Straw-* are always denied; Authorization/Cookie set-or-append operations
// require tenant_admin; op count is bounded. It returns the validated
// config.InjectionOperation list or an error message safe to surface as
// invalid_request.
func validateInjectionOperations(ops []injectionOperationBody, isTenantAdmin bool) ([]config.InjectionOperation, string) {
	if len(ops) > maxInjectionOperations {
		return nil, "operation count exceeds max_operations"
	}

	out := make([]config.InjectionOperation, 0, len(ops))

	for _, op := range ops {
		reason := validateInjectionOperation(op, isTenantAdmin)
		if reason != "" {
			return nil, reason
		}

		out = append(out, config.InjectionOperation{Op: op.Op, HeaderName: op.HeaderName, ValueBase64: op.ValueBase64})
	}

	return out, ""
}

// validateInjectionOperation checks one operation against the safety rules,
// returning "" when it is allowed.
func validateInjectionOperation(op injectionOperationBody, isTenantAdmin bool) string {
	if op.Op != injectionOpSet && op.Op != injectionOpAppend && op.Op != injectionOpRemove {
		return "op must be set, append, or remove"
	}

	if !isValidHTTPToken(op.HeaderName) {
		return "header_name is not a valid HTTP token"
	}

	lower := strings.ToLower(op.HeaderName)

	if alwaysDeniedInjectionHeaders[lower] || strings.HasPrefix(lower, deniedInjectionHeaderPrefix) {
		return "header " + op.HeaderName + " may not be set by an injection policy"
	}

	if sensitiveInjectionHeaders[lower] && op.Op != injectionOpRemove && !isTenantAdmin {
		return "only tenant_admin may set or append " + op.HeaderName
	}

	return ""
}

// ListInjectionPolicies handles GET /api/v1/config/injection-policies.
func (h *AdminHandlers) ListInjectionPolicies(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator, RoleViewer)
	if !ok {
		return
	}

	if h.InjectionPolicies == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	limit, offset := parsePagination(r)

	records, err := h.InjectionPolicies.ListInjectionPolicies(r.Context(), identity.TenantID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]injectionPolicyResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, toInjectionPolicyResponse(rec))
	}

	writeJSON(w, http.StatusOK, out)
}

// CreateInjectionPolicy handles POST /api/v1/config/injection-policies.
// Operators may write only non-sensitive operations; tenant_admin may write
// any (docs/planning/26, docs/planning/27).
func (h *AdminHandlers) CreateInjectionPolicy(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	id, err := newResourceID()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	h.upsertInjectionPolicy(w, r, identity, id)
}

// UpdateInjectionPolicy handles PUT /api/v1/config/injection-policies/{id}.
func (h *AdminHandlers) UpdateInjectionPolicy(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	h.upsertInjectionPolicy(w, r, identity, r.PathValue("id"))
}

func (h *AdminHandlers) upsertInjectionPolicy(w http.ResponseWriter, r *http.Request, identity Identity, id string) {
	if h.InjectionPolicies == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	var req injectionPolicyRequest

	err := decodeJSONBody(r, &req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	ops, reason := validateInjectionOperations(req.Operations, identity.Role == RoleTenantAdmin)
	if reason != "" {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: reason}))

		return
	}

	pol := config.InjectionPolicy{ID: id, Enabled: boolOrDefault(req.Enabled, true), Operations: ops}

	record, tenantVersion, err := h.InjectionPolicies.UpsertInjectionPolicy(r.Context(), identity.TenantID, pol, req.ExpectedConfigVersion, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, func(ctx context.Context) (uint64, error) {
			got, getErr := h.InjectionPolicies.GetInjectionPolicy(ctx, identity.TenantID, id)
			if getErr != nil {
				return 0, fmt.Errorf("get injection policy: %w", getErr)
			}

			return got.ConfigVersion, nil
		})

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "injection_policy", record.ID, configActionUpsert)

	writeJSON(w, http.StatusOK, toInjectionPolicyResponse(record))
}

// DeleteInjectionPolicy handles DELETE /api/v1/config/injection-policies/{id}.
func (h *AdminHandlers) DeleteInjectionPolicy(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator)
	if !ok {
		return
	}

	if h.InjectionPolicies == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	id := r.PathValue("id")

	tenantVersion, err := h.InjectionPolicies.DeleteInjectionPolicy(r.Context(), identity.TenantID, id, configActor(identity))
	if err != nil {
		h.writeConfigResourceError(r, w, err, nil)

		return
	}

	h.ConfigCache.PublishInvalidation(r.Context(), identity.TenantID, tenantVersion)
	recordAudit(r.Context(), h.Audit, identity, "injection_policy", id, configActionDelete)

	w.WriteHeader(http.StatusNoContent)
}

// ---- fingerprint profiles (read-only) ----

type fingerprintProfileResponse struct {
	Name              string `json:"name"`
	ScopeType         string `json:"scope_type"`
	SupportedByWorker bool   `json:"supported_by_worker"`
	Enabled           bool   `json:"enabled"`
	ConfigVersion     uint64 `json:"config_version"`
}

// ListFingerprintProfiles handles GET /api/v1/config/fingerprint-profiles.
// P0 has no write path: profiles are seeded built-ins (docs/planning/26).
func (h *AdminHandlers) ListFingerprintProfiles(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.authorizeConfig(w, r, RoleTenantAdmin, RoleOperator, RoleViewer)
	if !ok {
		return
	}

	if h.FingerprintProfiles == nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	records, err := h.FingerprintProfiles.ListFingerprintProfiles(r.Context(), identity.TenantID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	out := make([]fingerprintProfileResponse, 0, len(records))
	for _, rec := range records {
		out = append(out, fingerprintProfileResponse{
			Name: rec.Name, ScopeType: rec.ScopeType, SupportedByWorker: rec.SupportedByWorker,
			Enabled: rec.Enabled, ConfigVersion: rec.ConfigVersion,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// ---- shared helpers ----

// authorizeConfig authenticates the caller and requires tenant scope plus one
// of the allowed tenant roles, reusing the same check worker admin actions use
// (docs/planning/26 config resources are all tenant-scoped).
func (h *AdminHandlers) authorizeConfig(w http.ResponseWriter, r *http.Request, allowed ...Role) (Identity, bool) {
	identity, err := h.requireTenantRole(r, allowed...)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return Identity{}, false
	}

	return identity, true
}

// writeConfigResourceError maps a config-resource store error to its HTTP
// response. On a version conflict it looks up the resource's actual current
// version (via currentVersion, nil-safe) so the response carries
// details.current_config_version per docs/planning/26.
func (h *AdminHandlers) writeConfigResourceError(r *http.Request, w http.ResponseWriter, err error, currentVersion func(context.Context) (uint64, error)) {
	switch {
	case errors.Is(err, ErrConfigResourceVersionConflict):
		WriteError(w, http.StatusConflict, ErrorResponseFromCode(Conflict, "", conflictDetails(r.Context(), currentVersion)))
	case errors.Is(err, ErrConfigResourceNotFound):
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(TenantNotFound, "", nil))
	case errors.Is(err, errInjectionPolicyTooLarge):
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: "operation count exceeds max_operations"}))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

func conflictDetails(ctx context.Context, currentVersion func(context.Context) (uint64, error)) map[string]string {
	details := map[string]string{}

	if currentVersion == nil {
		return details
	}

	v, err := currentVersion(ctx)
	if err != nil {
		return details
	}

	details["current_config_version"] = strconv.FormatUint(v, 10)

	return details
}
