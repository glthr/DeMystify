# Contributing to DeMystify

We welcome your involvement in developing this project. Please follow these steps to make contributing as straightforward and transparent as possible.

## Report Bugs

We use [GitHub issues](https://github.com/glthr/DeMystify/issues) to track public bugs. Report a bug by opening a new issue.

## Open Pull Requests

**Any pull request containing original copyrighted assets will be immediately rejected.**

To propose modifications to the codebase, submit pull requests following these guidelines:

1. Fork the repo and create your branch from `main`.
2. Add tests if you have added code that should be tested.
3. Ensure the test suite passes (`go test ./...`)
4. Lint your code.
5. New contributor? Do not forget to add your name to `AUTHORS.md`
6. Provide clear commit messages: Write descriptive commit messages explaining each change's purpose. They should comply with the [Conventional Commits specification](https://www.conventionalcommits.org/en/v1.0.0/).

## Validation

Run these checks before opening a pull request:

```bash
go test -race ./...
go vet ./...
```

Pull requests run the same formatting, vet and test checks automatically, with the oldest supported Go version (`.github/workflows/ci.yml`). Graphviz is not installed there, so the checks that need `neato` only run locally.

To verify the original six-stack dataset locally, run `DEMYSTIFY_TEST_DATA="$MYST_DATA" go test -run TestOriginalMystImport .`, where `MYST_DATA` points to the converted stack directory. The test selects expected counts from the input script rather than the graph output. The game data is optional and must not be added to the repository.

To compare native import with XML produced from the same originals, set both `DEMYSTIFY_NATIVE_TEST_DATA` (the directory of original stack data forks) and `DEMYSTIFY_TEST_DATA` (the corresponding XML directory), then run `go test -run TestNativeMystParity .`. This checks the decoded objects, graph records and generated DOT. Native parser fixtures and fuzz seeds in the repository are synthetic.

Format changed Go files with `gofmt`. The tests use synthetic stack and card data, so they do not require a Myst CD-ROM. When Graphviz is installed, the integration tests also validate generated DOT with `neato`; those checks are skipped when it is unavailable.

When Node.js 18 or newer is available, the Go web exporter tests also run the viewer's synthetic DOM/canvas interaction tests. They can be run directly with `node --test renderer/web/viewer_test.cjs`. These exercise interaction logic; they do not replace a visual browser check.

Run the optional scripts' tests with `python3 -m unittest discover -s scripts -p 'test_*.py'`. They cover the media extractor's synthetic resource forks and output safety, and the publication framing; the framing tests are skipped unless the libraries in `scripts/requirements-pdf.txt` are installed. Original game images, banks, decoded catalogues and browser exports must stay outside Git; `generated/web` is ignored by default.

## License

By submitting code changes, you agree that your contributions will be licensed under the same MIT license as the rest of the project.

## Thank you!

We appreciate your interest in contributing to DeMystify and look forward to your contributions!
