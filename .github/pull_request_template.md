## Description
<!-- What does this PR change, and why? Link the issue if there is one. -->

## Type of change
- [ ] `feat` - new feature
- [ ] `fix` - bug fix
- [ ] `refactor` - no behavior change
- [ ] `docs` - documentation only
- [ ] `style` - formatting only
- [ ] `chore` - tooling, CI, dependencies
- [ ] Breaking change (flags, tool schemas, admin API, or default behavior)

## How was this tested?
<!-- Commands run, and anything checked by hand (e.g. against a real Ollama host or MCP client). -->

## Checklist
- [ ] `go build ./...`, `go vet ./...` and `go test ./...` pass
- [ ] `golangci-lint run ./...` passes
- [ ] `cd ui && npm run test && npm run build` pass (if `ui/` changed)
- [ ] Tests added or updated for the change
- [ ] README / `-h` help / tool descriptions / `skills/local-swarm/` updated if behavior changed
- [ ] No secrets, tokens, `.env` files, build output, or AI-assistant config in the diff

## Security impact
<!-- Does this touch auth, the admin API, process spawning, network exposure, or stored API keys? If so, how is it protected? Write "none" otherwise. -->
