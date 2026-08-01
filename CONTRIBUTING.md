# Contributing

## Getting set up

Don't have Go/Node/`act` installed? Run `make bootstrap` first — see the [README](README.md#quick-start).

```bash
make dev    # backend (go run) + frontend dev server, hot reload, Ctrl-C stops both
```

## Before opening a PR

```bash
make test   # go test ./... + npm test
make lint   # gofmt -l . && go vet ./... (check only)
make fmt    # gofmt -w . (formats in place)
```

Keep changes scoped — a bug fix shouldn't carry drive-by refactors. See `docs/ARCHITECTURE.md` for how the pieces fit together before touching something unfamiliar.

## Reporting bugs

Open a GitHub issue with: what you expected, what happened instead, and the workflow YAML or repro steps if the bug is workflow-parsing related. Screenshots help for UI bugs.

## Security issues

Don't open a public issue for a security vulnerability — see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree your contributions will be licensed under the project's [MIT License](LICENSE).
