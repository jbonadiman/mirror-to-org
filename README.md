# mirror-to-org
  
  Mirror GitHub/GitLab/Forgejo repositories into owner orgs on a Forgejo
  instance. Given one or more repository references, files each one into an
  organization named after its owner on the target instance, creating that
  organization from the owner's public profile when it does not already exist.
  Repositories land as live pull mirrors carrying their wiki.
  
  ## Install
  
  Requires Go 1.26+.
  
  ```bash
  go install github.com/jbonadiman/mirror-to-org@latest
  ```
  
  Or build from source:
  
  ```bash
  git clone https://github.com/jbonadiman/mirror-to-org
  cd mirror-to-org
  go build -o mirror-to-org .
  ```
  
  ## Usage
  
  ```bash
  export GITEA_TOKEN=...            # org create + repo migration rights on the target
  export GITEA_TARGET=https://your-forgejo-instance.example
  
  mirror-to-org github.com/kepano/obsidian-minimal
  mirror-to-org github.com/kepano/obsidian-minimal gitlab.com/some/repo
  mirror-to-org --target https://your-forgejo-instance.example github.com/kepano/obsidian-minimal
  ```
  
  A reference is a full URL or a `host/owner/repo` shorthand.
  
  ### Configuration
  
  | Flag / variable    | Required | Description                                  |
  |---------------------|----------|-----------------------------------------------|
  | `--target URL`       | one of `--target` or `$GITEA_TARGET` | Target instance base URL (must be `https://`) |
  | `$GITEA_TARGET`      | ^ | Same, as an env var; `--target` takes precedence |
  | `$GITEA_TOKEN`       | yes | Token with org create and repository migration rights on the target |
  
  ### Exit codes
  
  | Code | Meaning |
  |------|---------|
  | 0 | All references mirrored or skipped without error |
  | 1 | A runtime failure, or the target rejected the token |
  | 2 | Usage error (bad flags, no references, no target configured) |
  
  ## Development
  
  ```bash
  go build -o mirror-to-org .   # build the binary
  go test ./...                 # all tests
  go vet ./...                  # static checks
  gofmt -l .                    # formatting check (gofmt -w . to fix)
  ```
  
  See `AGENTS.md` for testing conventions and commit style.
  
  ## License
  
  AGPL-3.0 — see [`LICENSE`](LICENSE).
  