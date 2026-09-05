# AGENTS.md
  
  ## Project
  
  `mirror-to-org` mirrors GitHub/GitLab/Forgejo repositories into owner orgs
  on a Forgejo instance.
  
  ## Commands
  
  ```bash
  go build -o mirror-to-org .   # build the binary
  go test ./...                 # all tests
  go vet ./...                  # static checks
  gofmt -l .                    # formatting check (gofmt -w . to fix)
  ```
  
  ## Testing
  
  HTTP calls are mocked with an in-process `http.RoundTripper` function
  (`roundTripFunc` in each package's test file) rather than real sockets.
  Package-internal dependencies (target/source `*http.Client`) are passed
  as parameters, so tests substitute fakes without needing network access.
  
  ## Git
  
  Commit style: plain English imperative subject, body wrapped at 72
  columns, no conventional-commit prefixes.
  