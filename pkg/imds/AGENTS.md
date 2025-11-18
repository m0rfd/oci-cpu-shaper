# AGENTS

## Scope: `pkg/imds/`
- Constructors and option plumbing belong in `client_config.go`; keep the exported `Client` surface small and document new knobs with GoDoc.
- Add or change metadata lookups inside `operations.go`, keeping each helper focused on a single IMDSv2 endpoint (`GetInstance`, `GetShapeConfig`, etc.) plus matching unit tests in `operations_test.go`.
- HTTP/retry/backoff helpers live in `transport.go`; align timeouts and duty-cycle expectations with §§3 & 5 of the implementation plan and cover changes in `transport_test.go`.
- Share reusable fixtures via the existing `*_test_helpers.go` files instead of leaking them into production code.
- Follow the repo-wide QA rules referenced in the root AGENT: rerun `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` after edits, and update docs when behavior changes.
