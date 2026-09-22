package profile

import (
	"encoding/json"
	"regexp"
	"strings"
)

const GithubHost = "github.com"

var GitlabHosts = map[string]bool{"gitlab.com": true, "www.gitlab.com": true}

type Profile struct {
	Account     string
	FullName    string
	Email       string
	Website     string
	Description string
	Location    string
	AvatarURL   string
}

type GithubUser struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Description string `json:"description"`
	Blog        string `json:"blog"`
	Location    string `json:"location"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

type GiteaUser struct {
	Login       string `json:"login"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Location    string `json:"location"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

type GitlabProfile struct {
	Username    string `json:"username"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Description string `json:"description"`
	PublicEmail string `json:"public_email"`
	WebsiteURL  string `json:"website_url"`
	Location    string `json:"location"`
	AvatarURL   string `json:"avatar_url"`
}

func collapseSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// Matches both forges' synthesized noreply addresses.
var noreplyRE = regexp.MustCompile(`@([\w-]+\.)?noreply\.`)

func isNoreply(email string) bool {
	return email != "" && noreplyRE.MatchString(email)
}

func NormalizeWebsite(value string) string {
	if value == "" || strings.Contains(value, "://") {
		return value
	}
	return "https://" + value
}

func dropNoreply(email string) string {
	email = collapseSpace(email)
	if isNoreply(email) {
		return ""
	}
	return email
}

// normalize applies the cleaning every forge's raw profile needs, so the
// mappers below stay plain field renames.
func normalize(p Profile) Profile {
	p.FullName = collapseSpace(p.FullName)
	p.Email = dropNoreply(p.Email)
	p.Website = NormalizeWebsite(collapseSpace(p.Website))
	p.Description = collapseSpace(p.Description)
	p.Location = collapseSpace(p.Location)
	p.AvatarURL = collapseSpace(p.AvatarURL)
	return p
}

// MapGithubProfile: orgs are served from /users too, but use
// Description where users use Bio.
func MapGithubProfile(data GithubUser) Profile {
	description := collapseSpace(data.Bio)
	if description == "" {
		description = collapseSpace(data.Description)
	}
	return normalize(Profile{
		Account:     data.Login,
		FullName:    data.Name,
		Email:       data.Email,
		Website:     data.Blog,
		Description: description,
		Location:    data.Location,
		AvatarURL:   data.AvatarURL,
	})
}

func MapGiteaProfile(data GiteaUser) Profile {
	return normalize(Profile{
		Account:     data.Login,
		FullName:    data.FullName,
		Email:       data.Email,
		Website:     data.Website,
		Description: data.Description,
		Location:    data.Location,
		AvatarURL:   data.AvatarURL,
	})
}

// MapGitlabProfile: a user has Username/Bio, a group has Path/Description.
func MapGitlabProfile(data GitlabProfile) Profile {
	account := data.Username
	if account == "" {
		account = data.Path
	}
	description := collapseSpace(data.Bio)
	if description == "" {
		description = collapseSpace(data.Description)
	}
	return normalize(Profile{
		Account:     account,
		FullName:    data.Name,
		Email:       data.PublicEmail,
		Website:     data.WebsiteURL,
		Description: description,
		Location:    data.Location,
		AvatarURL:   data.AvatarURL,
	})
}

func MapProfile(host string, raw []byte) (Profile, error) {
	switch {
	case host == GithubHost:
		var data GithubUser
		if err := json.Unmarshal(raw, &data); err != nil {
			return Profile{}, err
		}
		return MapGithubProfile(data), nil
	case GitlabHosts[host]:
		var data GitlabProfile
		if err := json.Unmarshal(raw, &data); err != nil {
			return Profile{}, err
		}
		return MapGitlabProfile(data), nil
	default:
		var data GiteaUser
		if err := json.Unmarshal(raw, &data); err != nil {
			return Profile{}, err
		}
		return MapGiteaProfile(data), nil
	}
}
