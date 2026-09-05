// internal/reference/reference_test.go
package reference

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name      string
		ref       string
		wantHost  string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"full url with scheme", "https://github.com/kepano/minimal", "github.com", "kepano", "minimal", false},
		{"shorthand without scheme", "github.com/kepano/minimal", "github.com", "kepano", "minimal", false},
		{"http scheme is accepted", "http://github.com/kepano/minimal", "github.com", "kepano", "minimal", false},
		{"trailing slash is tolerated", "https://github.com/kepano/minimal/", "github.com", "kepano", "minimal", false},
		{"git suffix is stripped", "github.com/kepano/minimal.git", "github.com", "kepano", "minimal", false},
		{"host is lowercased", "GitHub.com/kepano/minimal", "github.com", "kepano", "minimal", false},
		{"non-github host is preserved", "codeberg.org/forgejo/forgejo", "codeberg.org", "forgejo", "forgejo", false},
		{"bare account raises", "kepano", "", "", "", true},
		{"profile reference raises", "github.com/kepano", "", "", "", true},
		{"extra path segments raise", "github.com/kepano/minimal/tree", "", "", "", true},
		{"empty string raises", "", "", "", "", true},
		{"hostless reference raises", "localhost/kepano/minimal", "", "", "", true},
		{"userinfo host raises", "https://github.com@evil.com/kepano/minimal", "", "", "", true},
		{"query string is stripped", "https://github.com/kepano/minimal?tab=readme", "github.com", "kepano", "minimal", false},
		{"fragment is stripped", "github.com/kepano/minimal#readme", "github.com", "kepano", "minimal", false},
		{"path traversal owner raises", "github.com/../../v1/admin/users/victim", "", "", "", true},
		{"host with port is accepted", "git.example.com:3000/kepano/minimal", "git.example.com:3000", "kepano", "minimal", false},
		{"ip literal host raises", "169.254.169.254/kepano/minimal", "", "", "", true},
		{"account case is preserved", "github.com/KePaNo/Minimal", "github.com", "KePaNo", "Minimal", false},
		{"dot segment repo name raises: .", "github.com/kepano/.", "", "", "", true},
		{"dot segment repo name raises: ..", "github.com/kepano/..", "", "", "", true},
		{"dot segment repo name raises: -", "github.com/kepano/-", "", "", "", true},
		{"bare git repo segment raises", "github.com/kepano/.git", "", "", "", true},
		{"repo may start with a dash", "github.com/kepano/-minimal", "github.com", "kepano", "-minimal", false},
		{"overlong repo name raises", "github.com/kepano/" + strings.Repeat("a", 101), "", "", "", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host, owner, repo, err := Parse(c.ref)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if host != c.wantHost || owner != c.wantOwner || repo != c.wantRepo {
				t.Fatalf("got (%q,%q,%q), want (%q,%q,%q)", host, owner, repo, c.wantHost, c.wantOwner, c.wantRepo)
			}
		})
	}
}

func TestValidateAccount(t *testing.T) {
	cases := []struct {
		name    string
		account string
		wantErr bool
	}{
		{"plain name passes", "kepano", false},
		{"name with dash and dot passes", "some-user.name", false},
		{"traversal raises", "../../v1/admin/users/victim", true},
		{"slash raises", "a/b", true},
		{"empty raises", "", true},
		{"leading dash raises", "-nope", true},
		{"repo-shaped leading dash raises", "-minimal", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateAccount(c.account)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got none")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
