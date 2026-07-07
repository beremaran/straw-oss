package control

import (
	"errors"
	"net/http"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
)

const mediaTypePEM = "application/x-pem-file"

// MITMCAHandler serves the operator-provided public CA certificate.
type MITMCAHandler struct {
	Authenticator *Authenticator
	ConfigCache   *ConfigCache
	CertFile      string
}

// ServeHTTP handles GET /api/v1/mitm/ca.pem.
func (h *MITMCAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	identity, err := h.Authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeMITMCAAuthError(w, err)

		return
	}

	if !CanExecuteDataPlane(identity) || !h.tenantAllowsMITM(r, identity) {
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))

		return
	}

	cert, err := os.ReadFile(h.CertFile)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	w.Header().Set(headerCanonicalContentType, mediaTypePEM)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cert)
}

func (h *MITMCAHandler) tenantAllowsMITM(r *http.Request, identity Identity) bool {
	if h.ConfigCache == nil {
		return false
	}

	snapshot, err := h.ConfigCache.Snapshot(r.Context(), identity.TenantID)
	if err != nil {
		return false
	}

	for _, rule := range snapshot.RoutingRules {
		if rule.Enabled && ruleAllowsMITM(rule) {
			return true
		}
	}

	return false
}

func ruleAllowsMITM(rule config.RoutingRule) bool {
	return rule.Match.IngressType == "" || rule.Match.IngressType == IngressTypeMITM
}

func writeMITMCAAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInsufficientPermissions):
		WriteError(w, http.StatusForbidden, ErrorResponseFromCode(InsufficientPermissions, "", nil))
	case errors.Is(err, ErrTenantNotFound):
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(TenantNotFound, "", nil))
	default:
		WriteError(w, http.StatusUnauthorized, ErrorResponseFromCode(AuthFailure, "", nil))
	}
}
