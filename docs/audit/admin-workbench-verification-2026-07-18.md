# Admin Workbench Verification Baseline

Date: 2026-07-18

The real-Control browser gate was run unchanged from `web`:

```bash
npx playwright test --config playwright.live-control.config.js
```

It stopped in Playwright global setup before any browser or application
assertion. The setup reached the real Control binary build and the local Go
toolchain rejected the repository module declaration:

```text
go version go1.19.8 linux/amd64
go: errors parsing go.mod:
go.mod:3: invalid go version '1.25.0': must match format 1.23
go.mod:5: unknown directive: toolchain
```

This is an environmental baseline, not an admin-workbench change. The feature
base commit `faa3bdd` already contains the same `go 1.25.0` and `toolchain
go1.26.5` declarations (`git diff faa3bdd..HEAD -- go.mod` is empty). A Go
toolchain compatible with the module declaration is required before this real
backend gate can reach its application assertions.
