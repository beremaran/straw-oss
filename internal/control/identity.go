package control

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/beremaran/straw/v2/internal/config"
)

const deploymentIdentityID = "deployment"

// ScopeType is retained in the internal request context for protocol
// compatibility. A self-hosted Straw process has exactly one scope.
type ScopeType string

// ScopeDeployment is the only supported request scope.
const ScopeDeployment ScopeType = "deployment"

// Identity is the deployment identity attached to an accepted request.
type Identity struct {
	APIKeyID  string
	ScopeType ScopeType
	TenantID  string
}

// ErrAuthFailure indicates a missing or incorrect deployment token.
var ErrAuthFailure = errors.New("auth failure")

// Authenticator checks the optional deployment-wide bearer token.
type Authenticator struct {
	token string
}

// NewDeploymentAuthenticator creates a deployment authenticator. An empty
// token allows requests, which is intended for loopback-only local use.
func NewDeploymentAuthenticator(token string) *Authenticator {
	return &Authenticator{token: token}
}

// Authenticate checks the request and returns the single deployment identity.
func (a *Authenticator) Authenticate(_ context.Context, authorizationHeader string) (Identity, error) {
	if a == nil {
		return Identity{}, ErrAuthFailure
	}

	if a.token != "" {
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authorizationHeader, bearerPrefix) {
			return Identity{}, ErrAuthFailure
		}

		provided := strings.TrimPrefix(authorizationHeader, bearerPrefix)
		if len(provided) != len(a.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(a.token)) != 1 {
			return Identity{}, ErrAuthFailure
		}
	}

	return Identity{
		APIKeyID:  deploymentIdentityID,
		ScopeType: ScopeDeployment,
		TenantID:  config.DefaultDeploymentID,
	}, nil
}
