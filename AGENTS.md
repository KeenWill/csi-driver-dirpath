# Working in this repo

Read `SPEC.md` first — it is the authority on scope and behavior. Keep this project lightweight: small PRs, plain prose, comments only where the code genuinely needs them. Do not import heavyweight process from other projects.

- Build: `go build ./...`
- Unit tests: `go test ./...`
- CSI conformance: `make sanity` (csi-sanity against the driver socket)
- e2e: `make e2e` (kind; see `hack/`)
- Lint/format: `gofmt`, `go vet`, `golangci-lint run` — all must be clean before committing.

Conventions: standard Go project layout (`cmd/`, `internal/`, `charts/`, `deploy/`, `hack/`). Sidecar images pinned by digest. Anything not covered by SPEC.md is an ordinary engineering judgment call — make it and note it in the PR description, not in a doc.
