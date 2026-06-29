package handlers

import (
	"testing"

	adminauth "github.com/beremaran/straw/internal/service/auth"
)

func TestRedirectWithTokens(t *testing.T) {
	tokens := &adminauth.TokenPair{
		AccessToken:  "access.token",
		RefreshToken: "refresh-token",
	}

	got, ok := redirectWithTokens("https://app.example/callback?next=/admin#old", tokens)
	if !ok {
		t.Fatal("redirectWithTokens rejected a valid URL")
	}

	want := "https://app.example/callback?next=/admin#access_token=access.token&refresh_token=refresh-token"
	if got.String() != want {
		t.Fatalf("redirectWithTokens() = %q, want %q", got.String(), want)
	}
}

func TestRedirectURLRejectsRelativeURL(t *testing.T) {
	if _, ok := redirectURL("/callback"); ok {
		t.Fatal("redirectURL accepted a relative URL")
	}
}
