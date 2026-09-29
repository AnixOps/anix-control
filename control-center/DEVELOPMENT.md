# AnixOps Control Center - Development Guide

## Branch Strategy

### Branch Types

| Branch | Purpose | Protection | Test Coverage |
|--------|---------|------------|---------------|
| `dev` | Development, RC versions | Moderate | 60% |
| `production` | Stable releases | Strict | 80% + Critical 100% |
| `v*.*.*` | Frozen versions | Full (immutable) | 80% + Critical 100% |

### Branch Protection Rules

#### `production` Branch
- ✅ Requires 2 approving reviews
- ✅ Requires CODEOWNER review
- ✅ Requires status checks: Lint, Test, Critical Tests, Build
- ✅ Requires linear history
- ✅ No force pushes
- ✅ No deletions
- ✅ Enforces admin restrictions

#### `dev` Branch
- ✅ Requires 1 approving review
- ✅ Requires status checks: Lint, Test, Build
- ✅ No force pushes
- ✅ No deletions

#### `v*.*.*` Branches (Frozen)
- ✅ Completely protected
- ✅ No direct commits allowed
- ✅ No PRs allowed
- ✅ Immutable after creation

---

## Version Management

### Semantic Versioning

```
MAJOR.MINOR.PATCH

MAJOR - Breaking changes
MINOR - New features (backward compatible)
PATCH - Bug fixes (backward compatible)
```

### Creating a Release

#### 1. Development Phase
```bash
# Work on dev branch
git checkout dev
git pull origin dev

# Make changes and test
make test
make test-coverage

# Bump version if needed
./scripts/version.sh bump minor
```

#### 2. Release Candidate
```bash
# Push to dev branch triggers RC build
git push origin dev

# GitHub Actions creates RC release automatically
# Version: v0.1.0-rc.123
```

#### 3. Production Release
```bash
# Merge to production
git checkout production
git merge dev --no-ff

# Push triggers production build
git push origin production

# GitHub Actions creates stable release
# Version: v0.2.0 (auto-incremented)
```

#### 4. Version Freeze
```bash
# Create frozen version branch
./scripts/version.sh freeze v0.2.0

# This creates:
# - Branch: v0.2.0
# - Tag: v0.2.0
# - Release: v0.2.0
```

---

## CI/CD Pipeline

### Workflow Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Push to Branch                            │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Quality Gates                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │   Lint   │→ │   Test   │→ │ Critical │→ │ Build All    │   │
│  │          │  │          │  │  Tests   │  │ Platforms    │   │
│  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Coverage Check                               │
│                                                                  │
│  dev branch:         60% minimum                                │
│  production branch:  80% minimum + 100% critical tests          │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Release                                   │
│                                                                  │
│  - Create GitHub Release                                         │
│  - Generate Release Notes                                        │
│  - Upload Artifacts (all platforms)                              │
│  - Generate Checksums                                            │
└─────────────────────────────────────────────────────────────────┘
```

### Quality Gates

#### Development Branch (60%)
```yaml
Required:
  - Lint passes
  - Tests pass
  - Coverage >= 60%
  - Build succeeds
```

#### Production Branch (80% + Critical 100%)
```yaml
Required:
  - Lint passes
  - Tests pass
  - Coverage >= 80%
  - Critical Tests: 100%
    - Plugin Manager tests
    - Auth/JWT tests
    - Config tests
  - Build succeeds
```

---

## Build Platforms

### Supported Platforms

| OS | Architecture | Binary | Archive |
|----|--------------|--------|---------|
| Linux | amd64 | anixops | .tar.gz |
| Linux | arm64 | anixops | .tar.gz |
| Windows | amd64 | anixops.exe | .zip |
| Windows | arm64 | anixops.exe | .zip |
| macOS | amd64 | anixops | .tar.gz |
| macOS | arm64 | anixops | .tar.gz |

### Mobile Support (Future)

| Platform | Status | Requirements |
|----------|--------|--------------|
| Android | Prepared | gomobile |
| iOS | Prepared | gomobile + Xcode |

### Building Locally

```bash
# Build for current platform
make build

# Build for all platforms
make build-release VERSION=v0.1.0

# Create release archives
make package-release VERSION=v0.1.0

# Full release process
make release VERSION=v0.1.0
```

---

## Development Workflow

### 1. Feature Development

```bash
# Create feature branch from dev
git checkout dev
git pull origin dev
git checkout -b feature/my-feature

# Develop and test
make test
make lint

# Push and create PR
git push origin feature/my-feature
# Create PR targeting dev branch
```

### 2. Bug Fix

```bash
# Create fix branch
git checkout dev
git checkout -b fix/issue-123

# Fix and test
make test
make test-critical

# Push and create PR
git push origin fix/issue-123
# Create PR targeting dev branch
```

### 3. Hot Fix (Production)

```bash
# Create hotfix branch from production
git checkout production
git checkout -b hotfix/critical-fix

# Fix and test
make test
make test-critical

# Push and create PRs
git push origin hotfix/critical-fix
# Create PRs targeting both dev and production
```

---

## Critical Functions

These functions must pass 100% for production releases:

### Plugin System
- `TestRegister` - Plugin registration
- `TestStartPlugin` - Plugin lifecycle
- `TestStopPlugin` - Plugin cleanup

### Authentication
- `TestValidateToken` - JWT validation
- `TestCheckPassword` - Password verification
- `TestHasPermission` - RBAC checks

### Configuration
- `TestDefaultConfig` - Default values
- `TestLoadFromString` - Config parsing

---

## Commercial Deployment

### Security Requirements

1. **Code Signing**
   - All releases must be signed
   - Checksums verified via SHA256/SHA512

2. **Audit Trail**
   - All commits signed
   - PR reviews required
   - Branch protection enforced

3. **Version Immutability**
   - Frozen version branches cannot be modified
   - All releases tagged and archived

### Release Checklist

- [ ] All tests pass
- [ ] Coverage meets threshold
- [ ] Critical tests 100%
- [ ] Lint passes
- [ ] CHANGELOG updated
- [ ] VERSION file updated
- [ ] Release notes prepared
- [ ] All platforms build successfully
- [ ] Checksums generated

---

## Important Commits Archive

Critical commits that should be preserved permanently:

| Commit | Date | Description |
|--------|------|-------------|
| `45d2d40` | 2026-03-15 | Initial implementation - Plugin system, API, TUI, Web GUI |

---

## Troubleshooting

### Common Issues

**Tests failing on CI but pass locally:**
- Check Go version matches
- Clear module cache: `go clean -modcache`
- Verify test isolation

**Build fails on specific platform:**
- Check CGO_ENABLED=0 for cross-compilation
- Verify no platform-specific dependencies
- Check architecture-specific code

**Coverage below threshold:**
- Run: `make test-coverage-summary`
- Identify uncovered code
- Add tests for critical paths

---

## Resources

- [GitHub Repository](https://github.com/AnixOps/anix-control/tree/go_dev/control-center) (`control-center/` in `AnixOps/anix-control`)
- [Releases](https://github.com/AnixOps/anix-control/releases) (Control Center releases use `control-center-v*` tags; older releases stay in the archived `AnixOps/Anixops-control-center` repository)
- [Actions](https://github.com/AnixOps/anix-control/actions/workflows/control-center.yml)