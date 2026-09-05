package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jbonadiman/mirror-to-org/internal/httperr"
	"github.com/jbonadiman/mirror-to-org/internal/profile"
	"github.com/jbonadiman/mirror-to-org/internal/reference"
	"github.com/jbonadiman/mirror-to-org/internal/target"
)

const helpText = `Mirror GitHub/GitLab/Forgejo repositories into owner orgs on a Forgejo instance

Usage:
  mirror-to-org [--target URL] REPO [REPO ...]

Arguments:
  REPO  Repository URL or host/owner/repo shorthand, e.g. github.com/kepano/obsidian-minimal

Options:
  --target URL  Target instance base URL (or set $GITEA_TARGET)
`

func resolveOwner(stdout io.Writer, client, source *http.Client, targetURL, host, owner string, resolved map[string]string) (string, bool, error) {
	if v, ok := resolved[owner]; ok {
		return v, false, nil
	}

	action, err := target.Probe(client, targetURL, owner)
	if err != nil {
		return "", false, err
	}
	if action != "create" {
		verdict := "ok"
		if action == "skip" {
			verdict = "skip"
		}
		resolved[owner] = verdict
		return verdict, false, nil
	}

	raw, err := profile.Fetch(source, host, owner)
	if err != nil {
		return "", false, err
	}
	prof, err := profile.MapProfile(host, raw)
	if err != nil {
		return "", false, err
	}
	// Must match the name we probed, or the collision check above guards
	// a different namespace than the one we're about to write.
	if !strings.EqualFold(prof.Account, owner) {
		return "", false, fmt.Errorf(
			"source reports account '%s' for owner '%s' — refusing to mirror into a different namespace",
			prof.Account, owner)
	}

	if err := target.CreateOrg(client, targetURL, target.BuildCreatePayload(prof, profile.SourceURL(host, owner))); err != nil {
		return "", false, err
	}
	fmt.Fprintf(stdout, "  Created org '%s'\n", owner)

	if prof.AvatarURL == "" {
		fmt.Fprintln(stdout, "  WARNING: source exposes no avatar")
	} else if err := target.UploadAvatar(client, source, targetURL, owner, prof.AvatarURL); err != nil {
		// Avatar is cosmetic; don't fail the reference over it.
		var credErr *httperr.CredentialError
		if errors.As(err, &credErr) {
			return "", false, err
		}
		fmt.Fprintf(stdout, "  WARNING: avatar upload failed: %v\n", err)
	}

	resolved[owner] = "ok"
	return "ok", true, nil
}

func mirrorOne(stdout io.Writer, client, source *http.Client, targetURL, ref string, resolved map[string]string) (status, detail string, orgCreated bool, err error) {
	host, owner, repo, err := reference.Parse(ref)
	if err != nil {
		return "", "", false, err
	}

	verdict, orgCreated, err := resolveOwner(stdout, client, source, targetURL, host, owner, resolved)
	if err != nil {
		return "", "", orgCreated, err
	}
	if verdict == "skip" {
		return "skip", fmt.Sprintf("'%s' is already a user account on the target", owner), orgCreated, nil
	}

	name := owner + "/" + repo
	exists, err := target.RepoExists(client, targetURL, owner, repo)
	if err != nil {
		return "", "", orgCreated, err
	}
	if exists {
		return "skip", fmt.Sprintf("'%s' is already mirrored", name), orgCreated, nil
	}

	migStatus, err := target.MigrateRepo(client, targetURL, target.BuildMigrationPayload(host, owner, repo))
	if err != nil {
		return "", "", orgCreated, err
	}
	if migStatus == "skip" {
		return "skip", fmt.Sprintf("'%s' is already mirrored", name), orgCreated, nil
	}

	fmt.Fprintf(stdout, "  Mirrored '%s'\n", name)
	return "ok", name, orgCreated, nil
}

func parseArgs(args []string) (targetURL string, references []string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("flag --target requires a value")
			}
			targetURL = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", nil, fmt.Errorf("unknown flag: %s", args[i])
			}
			references = append(references, args[i])
		}
	}
	if targetURL == "" {
		targetURL = os.Getenv("GITEA_TARGET")
	}
	return targetURL, references, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Run returns the process exit code: 0 success, 1 runtime/credential
// failure, 2 usage error.
func Run(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			fmt.Fprint(stdout, helpText)
			return 0
		}
	}

	targetURL, references, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	if len(references) == 0 {
		fmt.Fprintln(stderr, "at least one REPO argument is required")
		fmt.Fprint(stdout, helpText)
		return 2
	}

	if targetURL == "" {
		fmt.Fprintln(stderr, "Error: no target instance set.")
		fmt.Fprintln(stderr, "Pass --target URL or export $GITEA_TARGET with the instance's base URL.")
		return 2
	}

	if !strings.HasPrefix(targetURL, "https://") {
		fmt.Fprintln(stderr, "Error: --target must be an https:// URL.")
		return 1
	}

	token := os.Getenv("GITEA_TOKEN")
	if token == "" {
		fmt.Fprintln(stderr, "Error: $GITEA_TOKEN is not set.")
		fmt.Fprintln(stderr, "Export GITEA_TOKEN with org create and repository migration rights before running.")
		return 1
	}
	targetURL = strings.TrimSuffix(targetURL, "/")

	client := target.NewClient(token)
	client.Timeout = 30 * time.Second
	source := &http.Client{Timeout: 30 * time.Second}

	var mirrored, skipped, failed, orgs int
	resolved := map[string]string{}

	for _, ref := range references {
		fmt.Fprintf(stdout, "\n--- %s ---\n", ref)
		status, detail, created, err := mirrorOne(stdout, client, source, targetURL, ref, resolved)
		if err != nil {
			var credErr *httperr.CredentialError
			if errors.As(err, &credErr) {
				fmt.Fprintf(stderr, "  ERROR: %v\n", err)
				fmt.Fprintln(stderr, "Aborting: the target rejected the token.")
				return 1
			}
			fmt.Fprintf(stdout, "  ERROR: %v\n", err)
			failed++
			continue
		}

		orgs += boolToInt(created)
		if status == "ok" {
			mirrored++
		} else {
			fmt.Fprintf(stdout, "  WARNING: skipped — %s\n", detail)
			skipped++
		}
	}

	fmt.Fprintf(stdout, "\nDone. Processed %d: %d mirrored, %d skipped, %d failed. %d org(s) created.\n",
		len(references), mirrored, skipped, failed, orgs)

	if failed > 0 {
		return 1
	}
	return 0
}
