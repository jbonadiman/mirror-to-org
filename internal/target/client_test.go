// internal/target/client_test.go
package target

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jbonadiman/mirror-to-org/internal/httperr"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func textResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

const testTarget = "https://target.test"

var migrationBody = MigrationPayload{RepoOwner: "kepano", RepoName: "minimal", CloneAddr: "x"}

func TestNewClientRefusesForeignRedirect(t *testing.T) {
	var seen []string
	client := NewClient("SECRET")
	client.Transport = &authTransport{token: "SECRET", base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.String()+" auth="+r.Header.Get("Authorization"))
		resp := textResponse(302, "")
		resp.Header.Set("Location", "https://evil.test/steal")
		return resp, nil
	})}

	if _, _, err := doRequest(client, http.MethodGet, testTarget+"/api/v1/orgs/x", nil, nil); err == nil {
		t.Fatal("expected the redirect to be refused")
	}
	if len(seen) != 1 {
		t.Fatalf("expected the redirect not to be followed, got %v", seen)
	}
}

func TestRepoExists(t *testing.T) {
	t.Run("absent repository probes false", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(404, ""), nil })}
		ok, err := RepoExists(client, testTarget, "kepano", "minimal")
		if err != nil || ok {
			t.Fatalf("got %v, %v", ok, err)
		}
	})

	t.Run("existing repository probes true", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(200, "{}"), nil })}
		ok, err := RepoExists(client, testTarget, "kepano", "minimal")
		if err != nil || !ok {
			t.Fatalf("got %v, %v", ok, err)
		}
	})

	t.Run("probe error status raises", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(500, "boom"), nil })}
		if _, err := RepoExists(client, testTarget, "kepano", "minimal"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestMigrateRepo(t *testing.T) {
	t.Run("created migration reports ok", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(201, "{}"), nil })}
		status, err := MigrateRepo(client, testTarget, migrationBody)
		if err != nil || status != "ok" {
			t.Fatalf("got %q, %v", status, err)
		}
	})

	t.Run("conflict on an existing repository is a skip", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return textResponse(409, `{"message":"The repository with the same name already exists."}`), nil
		})}
		status, err := MigrateRepo(client, testTarget, migrationBody)
		if err != nil || status != "skip" {
			t.Fatalf("got %q, %v", status, err)
		}
	})

	t.Run("conflict on orphaned files is a failure", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return textResponse(409, `{"message":"Files already exist for this repository."}`), nil
		})}
		_, err := MigrateRepo(client, testTarget, migrationBody)
		if err == nil || !strings.Contains(err.Error(), "failed migration") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("unclonable source fails the reference only", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return textResponse(422, `{"message":"Migration failed: could not clone the source"}`), nil
		})}
		_, err := MigrateRepo(client, testTarget, migrationBody)
		var credErr *httperr.CredentialError
		if err == nil || errors.As(err, &credErr) || !strings.Contains(err.Error(), "could not clone") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("disabled migrations abort the run", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return textResponse(403, `{"message":"MigrationsGlobalDisabled"}`), nil
		})}
		_, err := MigrateRepo(client, testTarget, migrationBody)
		var credErr *httperr.CredentialError
		if !errors.As(err, &credErr) {
			t.Fatalf("expected CredentialError, got %v", err)
		}
	})

	t.Run("unauthorized migration is a credential error", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(401, "bad token"), nil })}
		_, err := MigrateRepo(client, testTarget, migrationBody)
		var credErr *httperr.CredentialError
		if !errors.As(err, &credErr) {
			t.Fatalf("expected CredentialError, got %v", err)
		}
	})

	t.Run("migration carries the token", func(t *testing.T) {
		var seenAuth string
		client := &http.Client{Transport: &authTransport{token: "SECRET", base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenAuth = r.Header.Get("Authorization")
			return textResponse(201, "{}"), nil
		})}}
		if _, err := MigrateRepo(client, testTarget, migrationBody); err != nil {
			t.Fatal(err)
		}
		if seenAuth != "token SECRET" {
			t.Fatalf("got %q", seenAuth)
		}
	})

	t.Run("migration uses its own longer timeout without mutating the caller's client", func(t *testing.T) {
		// net/http has no wire-level timeout introspection like httpx's
		// request.extensions, so this asserts the underlying guarantee
		// instead: MigrateRepo must not leave its longer timeout on the
		// shared client for subsequent calls.
		client := &http.Client{
			Timeout:   30 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(201, "{}"), nil }),
		}
		if _, err := MigrateRepo(client, testTarget, migrationBody); err != nil {
			t.Fatal(err)
		}
		if client.Timeout != 30*time.Second {
			t.Fatalf("expected caller's client Timeout untouched, got %v", client.Timeout)
		}
	})
}

func TestCreateOrg(t *testing.T) {
	t.Run("posts the body and succeeds on 2xx", func(t *testing.T) {
		var seenBody OrgCreatePayload
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &seenBody)
			return textResponse(201, "{}"), nil
		})}
		body := OrgCreatePayload{Username: "kepano", Visibility: "limited"}
		if err := CreateOrg(client, testTarget, body); err != nil {
			t.Fatal(err)
		}
		if seenBody.Username != "kepano" {
			t.Fatalf("got %+v", seenBody)
		}
	})
}

func TestUploadAvatar(t *testing.T) {
	t.Run("non-https avatar url is refused", func(t *testing.T) {
		client := &http.Client{}
		source := &http.Client{}
		err := UploadAvatar(client, source, testTarget, "kepano", "http://avatars.test/a.png")
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("ip literal avatar host is refused", func(t *testing.T) {
		fetched := false
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			fetched = true
			return nil, errors.New("unreachable")
		})}
		if err := UploadAvatar(&http.Client{}, source, testTarget, "kepano", "https://127.0.0.1/a.png"); err == nil {
			t.Fatal("expected an error")
		}
		if fetched {
			t.Fatal("avatar URL was fetched despite an IP literal host")
		}
	})

	t.Run("redirecting avatar url is refused", func(t *testing.T) {
		var urls []string
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			urls = append(urls, r.URL.String())
			resp := textResponse(302, "")
			resp.Header.Set("Location", "http://127.0.0.1/secret")
			return resp, nil
		})}
		if err := UploadAvatar(&http.Client{}, source, testTarget, "kepano", "https://avatars.test/a.png"); err == nil {
			t.Fatal("expected an error")
		}
		if len(urls) != 1 {
			t.Fatalf("expected the redirect not to be followed, got %v", urls)
		}
	})

	t.Run("downloads and base64-encodes the avatar", func(t *testing.T) {
		var seenImage string
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("\x89PNG")), Header: make(http.Header)}, nil
		})}
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(r.Body)
			var m map[string]string
			json.Unmarshal(b, &m)
			seenImage = m["image"]
			return textResponse(204, ""), nil
		})}
		if err := UploadAvatar(client, source, testTarget, "kepano", "https://avatars.test/a.png"); err != nil {
			t.Fatal(err)
		}
		if seenImage == "" {
			t.Fatal("expected a non-empty base64 image")
		}
	})
}

func TestProbe(t *testing.T) {
	t.Run("existing org updates", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/orgs/kepano") {
				return textResponse(200, "{}"), nil
			}
			return textResponse(404, ""), nil
		})}
		action, err := Probe(client, testTarget, "kepano")
		if err != nil || action != "update" {
			t.Fatalf("got %q, %v", action, err)
		}
	})

	t.Run("name taken by user skips", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/users/kepano") {
				return textResponse(200, "{}"), nil
			}
			return textResponse(404, ""), nil
		})}
		action, err := Probe(client, testTarget, "kepano")
		if err != nil || action != "skip" {
			t.Fatalf("got %q, %v", action, err)
		}
	})

	t.Run("unused name creates", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return textResponse(404, ""), nil })}
		action, err := Probe(client, testTarget, "kepano")
		if err != nil || action != "create" {
			t.Fatalf("got %q, %v", action, err)
		}
	})
}
