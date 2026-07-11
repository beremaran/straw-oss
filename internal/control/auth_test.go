package control

import (
	"context"
	"errors"
	"testing"
)

func TestDeploymentAuthenticatorAllowsAnonymousLocalRequests(t *testing.T) {
	t.Parallel()

	identity, err := NewDeploymentAuthenticator("").Authenticate(context.Background(), "")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if identity.TenantID != "default" || identity.ScopeType != ScopeDeployment {
		t.Fatalf("identity = %+v, want deployment identity", identity)
	}
}

func TestDeploymentAuthenticatorChecksConfiguredBearerToken(t *testing.T) {
	t.Parallel()

	auth := NewDeploymentAuthenticator("secret")
	_, err := auth.Authenticate(context.Background(), "Bearer wrong")
	if !errors.Is(err, ErrAuthFailure) {
		t.Fatalf("wrong token error = %v, want ErrAuthFailure", err)
	}

	_, err = auth.Authenticate(context.Background(), "Bearer secret")
	if err != nil {
		t.Fatalf("correct token error = %v", err)
	}
}
