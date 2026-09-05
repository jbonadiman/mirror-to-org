// internal/target/payload_test.go
package target

import (
	"encoding/json"
	"testing"

	"github.com/jbonadiman/mirror-to-org/internal/profile"
)

func TestServiceForHost(t *testing.T) {
	cases := []struct{ host, want string }{
		{"github.com", "github"},
		{"gitlab.com", "gitlab"},
		{"codeberg.org", "git"},
	}
	for _, c := range cases {
		if got := ServiceForHost(c.host); got != c.want {
			t.Errorf("ServiceForHost(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

func TestBuildMigrationPayload(t *testing.T) {
	t.Run("mirror and wiki are requested and repo is public", func(t *testing.T) {
		body := BuildMigrationPayload("github.com", "kepano", "minimal")
		if !body.Mirror || !body.Wiki || body.Private {
			t.Fatalf("got %+v", body)
		}
	})

	t.Run("lfs is requested", func(t *testing.T) {
		if !BuildMigrationPayload("github.com", "kepano", "minimal").LFS {
			t.Fatal("expected LFS true")
		}
	})

	t.Run("clone address is built from the reference", func(t *testing.T) {
		body := BuildMigrationPayload("git.example.com:3000", "kepano", "minimal")
		if body.CloneAddr != "https://git.example.com:3000/kepano/minimal" {
			t.Fatalf("got %q", body.CloneAddr)
		}
	})

	t.Run("owner and name come from the reference", func(t *testing.T) {
		body := BuildMigrationPayload("github.com", "KePaNo", "Minimal")
		if body.RepoOwner != "KePaNo" || body.RepoName != "Minimal" {
			t.Fatalf("got %+v", body)
		}
	})

	t.Run("github host maps to the github service", func(t *testing.T) {
		if BuildMigrationPayload("github.com", "kepano", "minimal").Service != "github" {
			t.Fatal("expected github service")
		}
	})

	t.Run("gitlab host maps to the gitlab service", func(t *testing.T) {
		if BuildMigrationPayload("gitlab.com", "kepano", "minimal").Service != "gitlab" {
			t.Fatal("expected gitlab service")
		}
	})

	t.Run("unrecognized host maps to a plain git clone", func(t *testing.T) {
		if BuildMigrationPayload("codeberg.org", "kepano", "minimal").Service != "git" {
			t.Fatal("expected git service")
		}
	})

	t.Run("body omits content the server clears on a mirror", func(t *testing.T) {
		// Struct fields with no json tag for issues/pull_requests/labels/
		// milestones/releases/mirror_interval mean marshaling never emits
		// them — there is nothing to omit at the Go type level.
		body, err := json.Marshal(BuildMigrationPayload("github.com", "kepano", "minimal"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"issues", "pull_requests", "labels", "milestones", "releases", "mirror_interval"} {
			if _, ok := m[key]; ok {
				t.Fatalf("expected %q to be absent", key)
			}
		}
	})
}

func TestDecideAction(t *testing.T) {
	cases := []struct {
		name                  string
		orgStatus, userStatus int
		want                  string
	}{
		{"existing org updates", 200, 0, "update"},
		{"unused name creates", 404, 404, "create"},
		{"name taken by user skips", 404, 200, "skip"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DecideAction(c.orgStatus, c.userStatus); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildCreatePayload(t *testing.T) {
	baseProfile := profile.Profile{
		Account:     "kepano",
		FullName:    "Steph Ango",
		Website:     "https://stephango.com",
		Description: "CEO of Obsidian",
		Location:    "Los Angeles",
	}

	t.Run("body includes username", func(t *testing.T) {
		if got := BuildCreatePayload(baseProfile, "https://github.com/kepano").Username; got != "kepano" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("visibility is set to limited", func(t *testing.T) {
		if got := BuildCreatePayload(baseProfile, "https://github.com/kepano").Visibility; got != "limited" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("missing website falls back to source url", func(t *testing.T) {
		p := baseProfile
		p.Website = ""
		if got := BuildCreatePayload(p, "https://github.com/kepano").Website; got != "https://github.com/kepano" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("existing website is kept", func(t *testing.T) {
		if got := BuildCreatePayload(baseProfile, "https://github.com/kepano").Website; got != "https://stephango.com" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty fields are omitted from the marshaled body", func(t *testing.T) {
		p := profile.Profile{Account: "kepano", Website: "https://stephango.com"}
		body, err := json.Marshal(BuildCreatePayload(p, "https://github.com/kepano"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"email", "location", "full_name", "description"} {
			if _, ok := m[key]; ok {
				t.Fatalf("expected %q to be absent, got %v", key, m)
			}
		}
	})

	t.Run("populated profile maps every field", func(t *testing.T) {
		p := baseProfile
		p.Email = "hi@example.com"
		body, err := json.Marshal(BuildCreatePayload(p, "https://github.com/kepano"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"username":    "kepano",
			"visibility":  "limited",
			"full_name":   "Steph Ango",
			"email":       "hi@example.com",
			"website":     "https://stephango.com",
			"description": "CEO of Obsidian",
			"location":    "Los Angeles",
		}
		if len(m) != len(want) {
			t.Fatalf("got %v, want %v", m, want)
		}
		for k, v := range want {
			if m[k] != v {
				t.Fatalf("key %q: got %v, want %v", k, m[k], v)
			}
		}
	})
}
