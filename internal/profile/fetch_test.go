package profile

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestFetch(t *testing.T) {
	t.Run("github uses the github api host", func(t *testing.T) {
		var seenURL string
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenURL = r.URL.String()
			return jsonResponse(200, `{"login":"kepano"}`), nil
		})}
		body, err := Fetch(client, GithubHost, "kepano")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seenURL != "https://api.github.com/users/kepano" {
			t.Fatalf("got url %q", seenURL)
		}
		if !strings.Contains(string(body), "kepano") {
			t.Fatalf("got body %q", body)
		}
	})

	t.Run("gitea-family host uses the users api", func(t *testing.T) {
		var seenURL string
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenURL = r.URL.String()
			return jsonResponse(200, `{"login":"forgejo"}`), nil
		})}
		if _, err := Fetch(client, "codeberg.org", "forgejo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seenURL != "https://codeberg.org/api/v1/users/forgejo" {
			t.Fatalf("got url %q", seenURL)
		}
	})

	t.Run("404 is ErrNotFound", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(404, ""), nil
		})}
		_, err := Fetch(client, "codeberg.org", "ghost")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("other error statuses are not ErrNotFound", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(500, "boom"), nil
		})}
		_, err := Fetch(client, "codeberg.org", "ghost")
		if err == nil || errors.Is(err, ErrNotFound) {
			t.Fatalf("expected a plain error, got %v", err)
		}
	})

	t.Run("gitlab user is found without probing groups", func(t *testing.T) {
		var paths []string
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			paths = append(paths, r.URL.Path)
			if r.URL.Path == "/api/v4/users" {
				return jsonResponse(200, `[{"username":"dzaporozhets"}]`), nil
			}
			return jsonResponse(404, ""), nil
		})}
		body, err := Fetch(client, "gitlab.com", "dzaporozhets")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(body), "dzaporozhets") {
			t.Fatalf("got body %q", body)
		}
		for _, p := range paths {
			if p == "/api/v4/groups/dzaporozhets" {
				t.Fatalf("group endpoint should not have been probed")
			}
		}
	})

	t.Run("gitlab falls back to the group endpoint on an empty user list", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v4/users" {
				return jsonResponse(200, `[]`), nil // empty list, not a 404
			}
			if r.URL.Path == "/api/v4/groups/inkscape" {
				return jsonResponse(200, `{"path":"inkscape"}`), nil
			}
			return jsonResponse(404, ""), nil
		})}
		body, err := Fetch(client, "gitlab.com", "inkscape")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(string(body), "inkscape") {
			t.Fatalf("got body %q", body)
		}
	})

	t.Run("gitlab name that is neither user nor group is ErrNotFound", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/api/v4/users" {
				return jsonResponse(200, `[]`), nil
			}
			return jsonResponse(404, ""), nil
		})}
		_, err := Fetch(client, "gitlab.com", "nope-zz9")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestSourceURL(t *testing.T) {
	if got := SourceURL("github.com", "kepano"); got != "https://github.com/kepano" {
		t.Fatalf("got %q", got)
	}
}
