package control

import (
	"bytes"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
)

const mediaTypePEM = "application/x-pem-file"

const mitmCAFileMode os.FileMode = 0o600

var (
	errDecodeMITMCACert = errors.New("decode mitm ca cert")
	errDecodeMITMCAKey  = errors.New("decode mitm ca key")
)

// MITMCAHandler serves the operator-provided public CA certificate.
type MITMCAHandler struct {
	Authenticator *Authenticator
	ConfigCache   *ConfigCache
	CertFile      string
	KeyFile       string
	Audit         AuditStore
}

type mitmCARotateRequest struct {
	CertPEM string `json:"cert_pem"`
	KeyPEM  string `json:"key_pem"`
}

type mitmCAResponse struct {
	CAIdentity string `json:"ca_identity"`
	CAVersion  string `json:"ca_version"`
}

// ServeHTTP handles GET /api/v1/mitm/ca.pem and PUT /api/v1/mitm/ca.
func (h *MITMCAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/mitm/ca.pem":
		h.servePublicCert(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/mitm/ca":
		h.rotateCA(w, r)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponseFromCode(InvalidRequest, "", nil))
	}
}

func (h *MITMCAHandler) servePublicCert(w http.ResponseWriter, r *http.Request) {
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

func (h *MITMCAHandler) rotateCA(w http.ResponseWriter, r *http.Request) {
	identity, err := h.Authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeMITMCAAuthError(w, err)

		return
	}

	err = RequireRole(identity, RoleTenantAdmin)
	if err != nil {
		writeMITMCAAuthError(w, err)

		return
	}

	var req mitmCARotateRequest

	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	cert, err := parseMITMCAPair([]byte(req.CertPEM), []byte(req.KeyPEM))
	if err != nil {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	err = writeMITMCAFilePair(h.CertFile, h.KeyFile, []byte(req.CertPEM), []byte(req.KeyPEM))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))

		return
	}

	identityID, version := mitmCAIdentityVersionForCert(cert)
	resp := mitmCAResponse{CAIdentity: identityID, CAVersion: version}
	recordAudit(r.Context(), h.Audit, identity, "mitm_ca", identity.TenantID, configActionUpdate, 0, auditFieldPathAll, nil, resp, false)
	writeJSON(w, http.StatusOK, resp)
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

func parseMITMCAPair(certPEM, keyPEM []byte) (*x509.Certificate, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, errDecodeMITMCACert
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse mitm ca cert: %w", err)
	}

	if !cert.IsCA {
		return nil, errDecodeMITMCACert
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errDecodeMITMCAKey
	}

	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse mitm ca key: %w", err)
		}
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errDecodeMITMCAKey
	}

	if !publicKeysEqual(cert.PublicKey, signer.Public()) {
		return nil, errDecodeMITMCAKey
	}

	return cert, nil
}

func publicKeysEqual(a, b any) bool {
	aDER, err := x509.MarshalPKIXPublicKey(a)
	if err != nil {
		return false
	}

	bDER, err := x509.MarshalPKIXPublicKey(b)
	if err != nil {
		return false
	}

	return bytes.Equal(aDER, bDER)
}

func mitmCAIdentityVersionForCert(cert *x509.Certificate) (string, string) {
	keySum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	certSum := sha256.Sum256(cert.Raw)

	return hex.EncodeToString(keySum[:]), hex.EncodeToString(certSum[:])
}

func writeMITMCAFilePair(certFile, keyFile string, certPEM, keyPEM []byte) error {
	if certFile == "" || keyFile == "" {
		return errDecodeMITMCAKey
	}

	err := os.WriteFile(certFile, certPEM, mitmCAFileMode)
	if err != nil {
		return fmt.Errorf("write mitm ca cert: %w", err)
	}

	err = os.WriteFile(keyFile, keyPEM, mitmCAFileMode)
	if err != nil {
		return fmt.Errorf("write mitm ca key: %w", err)
	}

	return nil
}
