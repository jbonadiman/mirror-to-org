package httperr

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckStatus(t *testing.T) {
	t.Run("success is silent", func(t *testing.T) {
		if err := CheckStatus(204, nil, "uploading avatar", false); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("error body is included", func(t *testing.T) {
		err := CheckStatus(422, []byte("name taken"), "creating org", false)
		if err == nil || !strings.Contains(err.Error(), "422") {
			t.Fatalf("expected error mentioning 422, got %v", err)
		}
	})

	t.Run("body appears in message", func(t *testing.T) {
		err := CheckStatus(422, []byte("name taken"), "creating org", false)
		if err == nil || !strings.Contains(err.Error(), "name taken") {
			t.Fatalf("expected error mentioning body, got %v", err)
		}
	})

	t.Run("long body is truncated", func(t *testing.T) {
		err := CheckStatus(500, []byte(strings.Repeat("x", 5000)), "creating org", false)
		if err == nil || len(err.Error()) >= 700 {
			t.Fatalf("expected truncated error under 700 chars, got %d", len(err.Error()))
		}
	})

	t.Run("target 401 is a credential error", func(t *testing.T) {
		err := CheckStatus(401, nil, "creating org", true)
		var credErr *CredentialError
		if !errors.As(err, &credErr) {
			t.Fatalf("expected *CredentialError, got %v", err)
		}
	})

	t.Run("target 403 is a credential error", func(t *testing.T) {
		err := CheckStatus(403, nil, "creating org", true)
		var credErr *CredentialError
		if !errors.As(err, &credErr) {
			t.Fatalf("expected *CredentialError, got %v", err)
		}
	})

	t.Run("source 403 is not a credential error", func(t *testing.T) {
		err := CheckStatus(403, nil, "fetching profile", false)
		var credErr *CredentialError
		if errors.As(err, &credErr) {
			t.Fatalf("expected plain error, got CredentialError")
		}
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}
