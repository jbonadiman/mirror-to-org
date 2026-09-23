package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
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

// verdict is how a reference ended: mirrored, skipped, or neither.
type verdict string

const (
	verdictOK   verdict = "ok"
	verdictSkip verdict = "skip"
)

// maxParallel caps how many references are mirrored at once. A migration is
// cloned server-side, so overlapping a few references hides that latency
// without hammering the target.
// ponytail: fixed fan-out; make it a flag if large batches need more.
const maxParallel = 4

// ownerCache memoizes the target probe (and any org creation) per owner, so
// a batch touches each owner once. The lock is held across resolution, which
// serializes different owners' probes; the migrations, the slow part, still
// overlap.
// ponytail: one lock for every owner; split per owner if profile fetching
// ever outgrows the migrations it overlaps.
type ownerCache struct {
	mu sync.Mutex
	m  map[string]verdict
}

func newOwnerCache() *ownerCache { return &ownerCache{m: map[string]verdict{}} }

// mirrorer runs references against one target. It keeps the clients, the
// target URL and the per-run owner cache together, so the steps below do
// not thread them through every call.
type mirrorer struct {
	client *http.Client
	source *http.Client
	target string
	stdout io.Writer
	owners *ownerCache
}

func (m *mirrorer) resolveOwner(host, owner string) (verdict, bool, error) {
	m.owners.mu.Lock()
	defer m.owners.mu.Unlock()

	if cached, ok := m.owners.m[owner]; ok {
		return cached, false, nil
	}

	action, err := target.Probe(m.client, m.target, owner)
	if err != nil {
		return "", false, err
	}
	switch action {
	case target.ActionSkip:
		m.owners.m[owner] = verdictSkip
		return verdictSkip, false, nil
	case target.ActionUpdate:
		m.owners.m[owner] = verdictOK
		return verdictOK, false, nil
	}

	raw, err := profile.Fetch(m.source, host, owner)
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

	if err := target.CreateOrg(m.client, m.target, target.BuildCreatePayload(prof, profile.SourceURL(host, owner))); err != nil {
		return "", false, err
	}
	fmt.Fprintf(m.stdout, "  Created org '%s'\n", owner)

	if prof.AvatarURL == "" {
		fmt.Fprintln(m.stdout, "  WARNING: source exposes no avatar")
	} else if err := target.UploadAvatar(m.client, m.source, m.target, owner, prof.AvatarURL); err != nil {
		// Avatar is cosmetic; don't fail the reference over it.
		var credErr *httperr.CredentialError
		if errors.As(err, &credErr) {
			return "", false, err
		}
		fmt.Fprintf(m.stdout, "  WARNING: avatar upload failed: %v\n", err)
	}

	m.owners.m[owner] = verdictOK
	return verdictOK, true, nil
}

func (m *mirrorer) mirror(ref string) (status verdict, detail string, orgCreated bool, err error) {
	host, owner, repo, err := reference.Parse(ref)
	if err != nil {
		return "", "", false, err
	}

	v, orgCreated, err := m.resolveOwner(host, owner)
	if err != nil {
		return "", "", orgCreated, err
	}
	if v == verdictSkip {
		return verdictSkip, fmt.Sprintf("'%s' is already a user account on the target", owner), orgCreated, nil
	}

	name := owner + "/" + repo
	exists, err := target.RepoExists(m.client, m.target, owner, repo)
	if err != nil {
		return "", "", orgCreated, err
	}
	if exists {
		return verdictSkip, fmt.Sprintf("'%s' is already mirrored", name), orgCreated, nil
	}

	migStatus, err := target.MigrateRepo(m.client, m.target, target.BuildMigrationPayload(host, owner, repo))
	if err != nil {
		return "", "", orgCreated, err
	}
	if migStatus == "skip" {
		return verdictSkip, fmt.Sprintf("'%s' is already mirrored", name), orgCreated, nil
	}

	fmt.Fprintf(m.stdout, "  Mirrored '%s'\n", name)
	return verdictOK, name, orgCreated, nil
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

// mirrorResult holds one reference's buffered output and outcome, so the
// caller can print every reference in the order it was given.
type mirrorResult struct {
	out        bytes.Buffer
	status     verdict
	detail     string
	orgCreated bool
	err        error
}

// mirrorAll mirrors every reference, running up to maxParallel at once, and
// prints each reference's output in the order the references were given. It
// returns the process exit code.
func mirrorAll(base *mirrorer, stdout, stderr io.Writer, references []string) int {
	results := make([]chan mirrorResult, len(references))
	sem := make(chan struct{}, maxParallel)
	for i, ref := range references {
		results[i] = make(chan mirrorResult, 1)
		go func(i int, ref string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			var out bytes.Buffer
			m := *base
			m.stdout = &out
			status, detail, created, err := m.mirror(ref)
			results[i] <- mirrorResult{out: out, status: status, detail: detail, orgCreated: created, err: err}
		}(i, ref)
	}

	var mirrored, skipped, failed, orgs int
	for i, ref := range references {
		r := <-results[i]
		fmt.Fprintf(stdout, "\n--- %s ---\n", ref)
		stdout.Write(r.out.Bytes())
		if r.err != nil {
			var credErr *httperr.CredentialError
			if errors.As(r.err, &credErr) {
				fmt.Fprintf(stderr, "  ERROR: %v\n", r.err)
				fmt.Fprintln(stderr, "Aborting: the target rejected the token.")
				return 1
			}
			fmt.Fprintf(stdout, "  ERROR: %v\n", r.err)
			failed++
			continue
		}

		if r.orgCreated {
			orgs++
		}
		if r.status == verdictOK {
			mirrored++
		} else {
			fmt.Fprintf(stdout, "  WARNING: skipped — %s\n", r.detail)
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

	m := &mirrorer{
		client: client,
		source: source,
		target: targetURL,
		stdout: stdout,
		owners: newOwnerCache(),
	}

	return mirrorAll(m, stdout, stderr, references)
}
