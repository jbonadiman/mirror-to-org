package profile

import "testing"

var githubResponse = GithubUser{
	Login:     "kepano",
	Name:      "Steph Ango",
	Bio:       "CEO of Obsidian",
	Blog:      "stephango.com",
	Location:  "Los Angeles",
	AvatarURL: "https://avatars.githubusercontent.com/u/1234",
}

var giteaResponse = GiteaUser{
	Login:       "forgejo",
	FullName:    "Forgejo",
	Description: "Beyond coding. We forge.",
	Website:     "https://forgejo.org",
	Location:    "",
	Email:       "forgejo@noreply.codeberg.org",
	AvatarURL:   "https://codeberg.org/avatars/abc",
}

var gitlabUserResponse = GitlabProfile{
	Username:    "dzaporozhets",
	Name:        "Dmytro Zaporozhets (DZ)",
	PublicEmail: "",
	AvatarURL:   "https://gitlab.com/uploads/-/system/user/avatar/444/avatar.png",
}

var gitlabGroupResponse = GitlabProfile{
	Name:        "Inkscape",
	Path:        "inkscape",
	Description: "Inkscape - Draw freely\r\n\r\nGet started: https://inkscape.org\r",
	AvatarURL:   "https://gitlab.com/uploads/-/system/group/avatar/470642/inkscape.png",
}

func TestNormalizeWebsite(t *testing.T) {
	cases := []struct{ in, want string }{
		{"stephango.com", "https://stephango.com"},
		{"https://forgejo.org", "https://forgejo.org"},
		{"http://example.com", "http://example.com"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeWebsite(c.in); got != c.want {
			t.Errorf("NormalizeWebsite(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapGithubProfile(t *testing.T) {
	t.Run("renames fields to normalized shape", func(t *testing.T) {
		result := MapGithubProfile(githubResponse)
		if result.Account != "kepano" || result.FullName != "Steph Ango" ||
			result.Description != "CEO of Obsidian" || result.Location != "Los Angeles" {
			t.Fatalf("got %+v", result)
		}
	})

	t.Run("schemeless blog gets a scheme", func(t *testing.T) {
		if got := MapGithubProfile(githubResponse).Website; got != "https://stephango.com" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty fields normalize to empty string", func(t *testing.T) {
		result := MapGithubProfile(GithubUser{Login: "ghost"})
		if result.FullName != "" || result.Description != "" || result.Website != "" || result.Email != "" {
			t.Fatalf("got %+v", result)
		}
		if result.Account != "ghost" {
			t.Fatalf("got account %q", result.Account)
		}
	})

	t.Run("real email is preserved", func(t *testing.T) {
		data := githubResponse
		data.Email = "hi@example.com"
		if got := MapGithubProfile(data).Email; got != "hi@example.com" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("github noreply email is dropped", func(t *testing.T) {
		data := githubResponse
		data.Email = "1+kepano@users.noreply.github.com"
		if got := MapGithubProfile(data).Email; got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("organization description is used", func(t *testing.T) {
		result := MapGithubProfile(GithubUser{Login: "obsidianmd", Description: "A knowledge base"})
		if result.Description != "A knowledge base" {
			t.Fatalf("got %q", result.Description)
		}
	})

	t.Run("user bio wins over description", func(t *testing.T) {
		data := githubResponse
		data.Description = "ignored"
		if got := MapGithubProfile(data).Description; got != "CEO of Obsidian" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestMapGiteaProfile(t *testing.T) {
	t.Run("passes native field names through", func(t *testing.T) {
		result := MapGiteaProfile(giteaResponse)
		if result.Account != "forgejo" || result.FullName != "Forgejo" ||
			result.Description != "Beyond coding. We forge." || result.Website != "https://forgejo.org" {
			t.Fatalf("got %+v", result)
		}
	})

	t.Run("empty strings normalize to empty", func(t *testing.T) {
		if got := MapGiteaProfile(giteaResponse).Location; got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("noreply email is dropped", func(t *testing.T) {
		if got := MapGiteaProfile(giteaResponse).Email; got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("numeric noreply email is dropped", func(t *testing.T) {
		data := giteaResponse
		data.Email = "38753+forgejo@noreply.gitea.com"
		if got := MapGiteaProfile(data).Email; got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("genuine email is preserved", func(t *testing.T) {
		data := giteaResponse
		data.Email = "hi@forgejo.org"
		if got := MapGiteaProfile(data).Email; got != "hi@forgejo.org" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestMapGitlabProfile(t *testing.T) {
	t.Run("user account comes from username", func(t *testing.T) {
		if got := MapGitlabProfile(gitlabUserResponse).Account; got != "dzaporozhets" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("group account comes from path", func(t *testing.T) {
		if got := MapGitlabProfile(gitlabGroupResponse).Account; got != "inkscape" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("user name maps to full name", func(t *testing.T) {
		if got := MapGitlabProfile(gitlabUserResponse).FullName; got != "Dmytro Zaporozhets (DZ)" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("multiline group description is collapsed to one line", func(t *testing.T) {
		got := MapGitlabProfile(gitlabGroupResponse).Description
		want := "Inkscape - Draw freely Get started: https://inkscape.org"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("empty public email is empty", func(t *testing.T) {
		if got := MapGitlabProfile(gitlabUserResponse).Email; got != "" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("real public email is kept", func(t *testing.T) {
		data := gitlabUserResponse
		data.PublicEmail = "hi@example.com"
		if got := MapGitlabProfile(data).Email; got != "hi@example.com" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("absent fields are empty", func(t *testing.T) {
		result := MapGitlabProfile(gitlabUserResponse)
		if result.Description != "" || result.Location != "" || result.Website != "" {
			t.Fatalf("got %+v", result)
		}
	})

	t.Run("user bio is used when present", func(t *testing.T) {
		data := gitlabUserResponse
		data.Bio = "Co-founder"
		if got := MapGitlabProfile(data).Description; got != "Co-founder" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("website_url gets a scheme", func(t *testing.T) {
		data := gitlabUserResponse
		data.WebsiteURL = "example.com"
		if got := MapGitlabProfile(data).Website; got != "https://example.com" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestMapProfile(t *testing.T) {
	t.Run("github host uses github mapper", func(t *testing.T) {
		raw := []byte(`{"login":"kepano","bio":"CEO of Obsidian"}`)
		result, err := MapProfile("github.com", raw)
		if err != nil || result.Description != "CEO of Obsidian" {
			t.Fatalf("got %+v, err %v", result, err)
		}
	})

	t.Run("other host uses gitea mapper", func(t *testing.T) {
		raw := []byte(`{"login":"forgejo","description":"Beyond coding. We forge."}`)
		result, err := MapProfile("codeberg.org", raw)
		if err != nil || result.Description != "Beyond coding. We forge." {
			t.Fatalf("got %+v, err %v", result, err)
		}
	})

	t.Run("gitlab host uses gitlab mapper", func(t *testing.T) {
		raw := []byte(`{"username":"dzaporozhets"}`)
		result, err := MapProfile("gitlab.com", raw)
		if err != nil || result.Account != "dzaporozhets" {
			t.Fatalf("got %+v, err %v", result, err)
		}
	})

	t.Run("gitlab www host uses gitlab mapper", func(t *testing.T) {
		raw := []byte(`{"path":"inkscape"}`)
		result, err := MapProfile("www.gitlab.com", raw)
		if err != nil || result.Account != "inkscape" {
			t.Fatalf("got %+v, err %v", result, err)
		}
	})

	t.Run("self-hosted gitlab host is not special-cased", func(t *testing.T) {
		// Not detectable from the hostname, so it falls through to the
		// Gitea shape like any other unknown host.
		raw := []byte(`{"login":"forgejo","description":"Beyond coding. We forge."}`)
		result, err := MapProfile("gitlab.example.com", raw)
		if err != nil || result.Description != "Beyond coding. We forge." {
			t.Fatalf("got %+v, err %v", result, err)
		}
	})
}
