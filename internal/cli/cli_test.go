// internal/cli/cli_test.go
package cli

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

const testTarget = "https://target.test"

// clients builds (client, source) where the target answers from a route
// table keyed by "METHOD path", recording every request it sees.
func clients(t *testing.T, routes map[string]*http.Response) (*http.Client, *http.Client, *[]string) {
	t.Helper()
	sent := &[]string{}
	targetHandler := func(r *http.Request) (*http.Response, error) {
		key := r.Method + " " + r.URL.Path
		*sent = append(*sent, key)
		if resp, ok := routes[key]; ok {
			return resp, nil
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/avatar") {
			return jsonResponse(204, ""), nil
		}
		if r.URL.Path == "/api/v1/repos/migrate" {
			return jsonResponse(201, "{}"), nil
		}
		if r.Method == http.MethodPost {
			return jsonResponse(200, `{"name":"kepano"}`), nil
		}
		return jsonResponse(404, ""), nil // GET probes: name is unused
	}
	sourceHandler := func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/avatar.png") {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("\x89PNG fake")), Header: make(http.Header)}, nil
		}
		return jsonResponse(200, `{"login":"kepano","name":"Steph Ango","bio":"CEO of Obsidian","blog":"stephango.com","location":"Los Angeles","avatar_url":"https://avatars.test/avatar.png"}`), nil
	}
	return &http.Client{Transport: roundTripFunc(targetHandler)}, &http.Client{Transport: roundTripFunc(sourceHandler)}, sent
}

func newMirrorer(client, source *http.Client, stdout io.Writer) *mirrorer {
	return &mirrorer{client: client, source: source, target: testTarget, stdout: stdout, owners: newOwnerCache()}
}

func TestMirrorOne(t *testing.T) {
	t.Run("unused name creates the org and mirrors", func(t *testing.T) {
		client, source, sent := clients(t, nil)
		var buf bytes.Buffer
		status, name, created, err := newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if err != nil || status != "ok" || name != "kepano/minimal" || !created {
			t.Fatalf("got %q %q %v %v", status, name, created, err)
		}
		if !contains(*sent, "POST /api/v1/orgs") || !contains(*sent, "POST /api/v1/repos/migrate") {
			t.Fatalf("got sent %v", *sent)
		}
	})

	t.Run("existing org is left untouched", func(t *testing.T) {
		client, source, sent := clients(t, map[string]*http.Response{"GET /api/v1/orgs/kepano": jsonResponse(200, `{"name":"kepano"}`)})
		var buf bytes.Buffer
		status, name, created, err := newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if err != nil || status != "ok" || name != "kepano/minimal" || created {
			t.Fatalf("got %q %q %v %v", status, name, created, err)
		}
		if contains(*sent, "POST /api/v1/orgs") {
			t.Fatalf("expected no org creation, got %v", *sent)
		}
	})

	t.Run("name taken by user writes nothing", func(t *testing.T) {
		client, source, sent := clients(t, map[string]*http.Response{"GET /api/v1/users/kepano": jsonResponse(200, `{"login":"kepano"}`)})
		var buf bytes.Buffer
		status, _, _, err := newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if err != nil || status != "skip" {
			t.Fatalf("got %q, %v", status, err)
		}
		for _, s := range *sent {
			if strings.HasPrefix(s, "POST") || strings.HasPrefix(s, "PATCH") {
				t.Fatalf("expected no writes, got %v", *sent)
			}
		}
	})

	t.Run("already mirrored repository issues no migration", func(t *testing.T) {
		client, source, sent := clients(t, map[string]*http.Response{
			"GET /api/v1/orgs/kepano":          jsonResponse(200, "{}"),
			"GET /api/v1/repos/kepano/minimal": jsonResponse(200, "{}"),
		})
		var buf bytes.Buffer
		status, detail, _, err := newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if err != nil || status != "skip" || !strings.Contains(detail, "already mirrored") {
			t.Fatalf("got %q %q %v", status, detail, err)
		}
		if contains(*sent, "POST /api/v1/repos/migrate") {
			t.Fatalf("expected no migration, got %v", *sent)
		}
	})

	t.Run("one owner is probed once across a batch", func(t *testing.T) {
		client, source, sent := clients(t, nil)
		var buf bytes.Buffer
		m := newMirrorer(client, source, &buf)
		for _, repo := range []string{"a", "b", "c"} {
			m.mirror("github.com/kepano/" + repo)
		}
		if countMatches(*sent, "GET /api/v1/orgs/kepano") != 1 {
			t.Fatalf("expected one org probe, got %v", *sent)
		}
		if countMatches(*sent, "POST /api/v1/orgs") != 1 {
			t.Fatalf("expected one org creation, got %v", *sent)
		}
		if countMatches(*sent, "POST /api/v1/repos/migrate") != 3 {
			t.Fatalf("expected three migrations, got %v", *sent)
		}
	})

	t.Run("profile naming another account creates nothing", func(t *testing.T) {
		client, _, sent := clients(t, nil)
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return jsonResponse(200, `{"login":"someone-else"}`), nil
		})}
		var buf bytes.Buffer
		_, _, _, err := newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if err == nil || !strings.Contains(err.Error(), "different namespace") {
			t.Fatalf("got %v", err)
		}
		if contains(*sent, "POST /api/v1/orgs") || contains(*sent, "POST /api/v1/repos/migrate") {
			t.Fatalf("expected no writes, got %v", *sent)
		}
	})

	t.Run("missing source profile raises a not-found error", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(404, ""), nil })}
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) { return jsonResponse(404, ""), nil })}
		var buf bytes.Buffer
		_, _, _, err := newMirrorer(client, source, &buf).mirror("github.com/ghost/nope")
		if err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("target token is never sent to the source", func(t *testing.T) {
		var seenAuth string
		source := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			seenAuth = r.Header.Get("Authorization")
			return jsonResponse(200, `{"login":"kepano"}`), nil
		})}
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method == http.MethodPost {
				return jsonResponse(200, `{"name":"kepano"}`), nil
			}
			return jsonResponse(404, ""), nil
		})}
		client.Transport = &authInjectingTransport{token: "SECRET", base: client.Transport}
		var buf bytes.Buffer
		newMirrorer(client, source, &buf).mirror("github.com/kepano/minimal")
		if seenAuth != "" {
			t.Fatalf("expected no Authorization header at the source, got %q", seenAuth)
		}
	})
}

func TestMirrorAllOverlapsMigrations(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak, orgCreates := 0, 0, 0

	targetClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs" {
			mu.Lock()
			orgCreates++
			mu.Unlock()
			return jsonResponse(200, "{}"), nil
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/migrate" {
			mu.Lock()
			inFlight++
			if inFlight > peak {
				peak = inFlight
			}
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			return jsonResponse(201, "{}"), nil
		}
		if r.Method == http.MethodPost {
			return jsonResponse(200, "{}"), nil
		}
		return jsonResponse(404, ""), nil
	})}
	sourceClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"login":"kepano"}`), nil
	})}

	refs := []string{"github.com/kepano/a", "github.com/kepano/b", "github.com/kepano/c", "github.com/kepano/d"}
	var stdout, stderr bytes.Buffer
	if code := mirrorAll(newMirrorer(targetClient, sourceClient, nil), &stdout, &stderr, refs); code != 0 {
		t.Fatalf("got exit %d, stderr %q", code, stderr.String())
	}

	mu.Lock()
	got := peak
	creates := orgCreates
	mu.Unlock()
	if got < 2 {
		t.Fatalf("expected migrations to overlap, peak concurrency was %d", got)
	}
	if creates != 1 {
		t.Fatalf("expected one org creation for four refs, got %d", creates)
	}

	out := stdout.String()
	for i, ref := range refs {
		if !strings.Contains(out, "--- "+ref+" ---") {
			t.Fatalf("missing section for %s in %q", ref, out)
		}
		if i > 0 && strings.Index(out, "--- "+refs[i-1]+" ---") > strings.Index(out, "--- "+ref+" ---") {
			t.Fatalf("sections out of order in %q", out)
		}
	}
	if !strings.Contains(out, "4 mirrored") {
		t.Fatalf("expected four mirrored, got %q", out)
	}
}

// authInjectingTransport stands in for target.NewClient's real transport
// without importing internal/target, keeping this test package focused
// on cli's own routing logic.
type authInjectingTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authInjectingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "token "+t.token)
	return t.base.RoundTrip(r)
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func countMatches(list []string, want string) int {
	n := 0
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}

func TestRun(t *testing.T) {
	t.Run("help describes repository references", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"--help"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("got exit %d", code)
		}
		if !strings.Contains(stdout.String(), "host/owner/repo") {
			t.Fatalf("got %q", stdout.String())
		}
		if strings.Contains(strings.ToLower(stdout.String()), "profiles") {
			t.Fatalf("got %q", stdout.String())
		}
	})

	t.Run("missing token exits one naming the variable", func(t *testing.T) {
		old, had := os.LookupEnv("GITEA_TOKEN")
		os.Unsetenv("GITEA_TOKEN")
		defer func() {
			if had {
				os.Setenv("GITEA_TOKEN", old)
			}
		}()
		var stdout, stderr bytes.Buffer
		code := Run([]string{"--target", "https://example.test", "github.com/kepano/minimal"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("got exit %d", code)
		}
		if !strings.Contains(stderr.String(), "GITEA_TOKEN") {
			t.Fatalf("got %q", stderr.String())
		}
	})

	t.Run("no arguments is a usage error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := Run(nil, &stdout, &stderr); code != 2 {
			t.Fatalf("got exit %d", code)
		}
	})

	t.Run("missing target exits two naming the flag and the variable", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"github.com/kepano/minimal"}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("got exit %d", code)
		}
		if !strings.Contains(stderr.String(), "--target") || !strings.Contains(stderr.String(), "GITEA_TARGET") {
			t.Fatalf("got %q", stderr.String())
		}
	})

	t.Run("GITEA_TARGET supplies the target when --target is absent", func(t *testing.T) {
		t.Setenv("GITEA_TARGET", "http://insecure.test")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"github.com/kepano/minimal"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("got exit %d", code)
		}
		if !strings.Contains(stderr.String(), "https://") {
			t.Fatalf("got %q", stderr.String())
		}
	})

	t.Run("a bad reference does not abandon the rest", func(t *testing.T) {
		t.Setenv("GITEA_TOKEN", "x")
		var stdout, stderr bytes.Buffer
		code := Run([]string{"--target", "https://example.test", "not-a-reference", "github.com/kepano"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("got exit %d", code)
		}
		if !strings.Contains(stdout.String(), "Processed 2") || !strings.Contains(stdout.String(), "2 failed") {
			t.Fatalf("got %q", stdout.String())
		}
	})
}
