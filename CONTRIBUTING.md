# Contributing

Thanks for your interest - contributions are welcome.

## Building and testing

Requires Go 1.25+.

```bash
go build ./...
go test ./...      # add -race when touching concurrency
go vet ./...
golangci-lint run ./...   # golangci-lint v2
```

CI runs the tests (with `-race`), `go vet`, and the linter on every
push and pull request. A release only publishes if all of them pass.

## Pull requests

- Keep each PR to one logical change.
- Add or update tests for any behavior change.
- Run the full gate above first - the PR must be green.
- Commit subjects: conventional, lowercase, present tense, under ~72
  chars (`feat:`, `fix:`, `docs:`, `ci:`, ...). Keep bodies short.

## A note on the Mojang / Xbox APIs

This project speaks to first-party Minecraft HTTPS endpoints. The wire
format (URL paths, JSON field names, header values) is observed protocol
and fine to implement from scratch. Please **do not copy code** from
GPL/AGPL/LGPL projects into this MIT codebase - write the structs and
helpers yourself from the observed traffic.

Realms is intentionally out of scope (paid Microsoft content with
stricter third-party-client terms).
