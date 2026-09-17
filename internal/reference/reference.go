package reference

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	accountRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,38}$`)
	hostRE    = regexp.MustCompile(`^[a-z0-9.-]+(:\d+)?$`)
	repoRE    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
	// Matches an IPv4 literal in every form inet_aton accepts: dotted
	// quad, short forms like 127.1, and octal/hex octets. A hostname's
	// last label is alphabetic, so this never matches one.
	ipLiteralRE = regexp.MustCompile(`^(0[xX][0-9a-fA-F]+|[0-9]+)(\.(0[xX][0-9a-fA-F]+|[0-9]+))*\.?$`)
)

var reservedRepoNames = map[string]bool{".": true, "..": true, "-": true}
var reservedRepoSuffixes = []string{".git", ".wiki", ".rss", ".atom"}

// ValidateAccount blocks names that could escape the API path —
// untrusted input coming from the source.
func ValidateAccount(name string) error {
	if !accountRE.MatchString(name) {
		return fmt.Errorf("invalid account name: %q", name)
	}
	return nil
}

// ValidateRepo is looser than ValidateAccount, but still blocks a
// '.'/'..' segment, which would make a missing repo look confirmed.
func ValidateRepo(name string) error {
	if !repoRE.MatchString(name) {
		return fmt.Errorf("invalid repository name: %q", name)
	}
	lower := strings.ToLower(name)
	if reservedRepoNames[name] {
		return fmt.Errorf("reserved repository name: %q", name)
	}
	for _, suffix := range reservedRepoSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return fmt.Errorf("reserved repository name: %q", name)
		}
	}
	return nil
}

// ValidateHost blocks hosts the target would fetch itself — an SSRF
// guard on the clone address.
func ValidateHost(host string) error {
	if strings.Contains(host, "@") || !strings.Contains(host, ".") || !hostRE.MatchString(host) {
		return fmt.Errorf("%q is not a valid host", host)
	}
	hostPart := strings.SplitN(host, ":", 2)[0]
	if ipLiteralRE.MatchString(hostPart) {
		return fmt.Errorf("refusing IP literal host: %s", host)
	}
	return nil
}

func Parse(ref string) (host, owner, repo string, err error) {
	cleaned := strings.TrimSpace(ref)
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(cleaned, scheme) {
			cleaned = cleaned[len(scheme):]
			break
		}
	}
	if idx := strings.IndexAny(cleaned, "?#"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSuffix(cleaned, "/")

	var parts []string
	for _, p := range strings.Split(cleaned, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf(
			"cannot parse %q — expected host/owner/repo, e.g. github.com/kepano/obsidian-minimal", ref)
	}

	host, owner, repo = parts[0], parts[1], parts[2]
	host = strings.ToLower(host)
	if verr := ValidateHost(host); verr != nil {
		return "", "", "", fmt.Errorf("cannot parse %q — %w", ref, verr)
	}

	repo = strings.TrimSuffix(repo, ".git")
	if verr := ValidateAccount(owner); verr != nil {
		return "", "", "", verr
	}
	if verr := ValidateRepo(repo); verr != nil {
		return "", "", "", verr
	}
	return host, owner, repo, nil
}
