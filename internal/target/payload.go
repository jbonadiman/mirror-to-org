package target

import (
	"fmt"

	"github.com/jbonadiman/mirror-to-org/internal/profile"
)

const defaultVisibility = "limited"

type OrgCreatePayload struct {
	Username    string `json:"username"`
	Visibility  string `json:"visibility"`
	FullName    string `json:"full_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Website     string `json:"website,omitempty"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
}

type MigrationPayload struct {
	CloneAddr string `json:"clone_addr"`
	RepoName  string `json:"repo_name"`
	RepoOwner string `json:"repo_owner"`
	Service   string `json:"service"`
	Mirror    bool   `json:"mirror"`
	LFS       bool   `json:"lfs"`
	Wiki      bool   `json:"wiki"`
	Private   bool   `json:"private"`
}

func ServiceForHost(host string) string {
	if host == profile.GithubHost {
		return "github"
	}
	if profile.GitlabHosts[host] {
		return "gitlab"
	}
	return "git"
}

// Only sets the flags that survive a mirror — the server clears the
// rest when mirror is set.
func BuildMigrationPayload(host, owner, repo string) MigrationPayload {
	return MigrationPayload{
		CloneAddr: fmt.Sprintf("https://%s/%s/%s", host, owner, repo),
		RepoName:  repo,
		RepoOwner: owner,
		Service:   ServiceForHost(host),
		Mirror:    true,
		LFS:       true,
		Wiki:      true,
		Private:   false,
	}
}

// Visibility is explicit: these mirror other people's profiles, not
// the instance owner's.
func BuildCreatePayload(p profile.Profile, sourceURL string) OrgCreatePayload {
	website := p.Website
	if website == "" {
		website = sourceURL
	}
	return OrgCreatePayload{
		Username:    p.Account,
		Visibility:  defaultVisibility,
		FullName:    p.FullName,
		Email:       p.Email,
		Website:     website,
		Description: p.Description,
		Location:    p.Location,
	}
}

// Gitea has no org/user discriminator, so a taken name shows as
// orgs-404 + users-200.
func DecideAction(orgStatus, userStatus int) string {
	if orgStatus == 200 {
		return "update"
	}
	if userStatus == 200 {
		return "skip"
	}
	return "create"
}
