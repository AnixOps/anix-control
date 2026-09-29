# Flutter Release Configuration

This document explains how to set up GitHub Secrets for automated Flutter releases.

## Required GitHub Secrets

Go to your repository → Settings → Secrets and variables → Actions → New repository secret

### Android Signing

| Secret Name | Description |
|-------------|-------------|
| `ANDROID_KEYSTORE_BASE64` | Base64 encoded keystore file |
| `ANDROID_KEY_ALIAS` | Key alias name |
| `ANDROID_KEY_PASSWORD` | Key password |
| `ANDROID_STORE_PASSWORD` | Keystore store password |

#### How to encode keystore to Base64:

```bash
# On Linux/macOS
base64 -i mobile/android/keystore/anixops-release.jks | pbcopy

# On Windows (PowerShell)
[Convert]::ToBase64String([IO.File]::ReadAllBytes("mobile\android\keystore\anixops-release.jks"))
```

### Example values:

```
ANDROID_KEY_ALIAS=anixops
ANDROID_KEY_PASSWORD=<your-key-password>
ANDROID_STORE_PASSWORD=<your-store-password>
```

## Manual Build Commands

If you need to build locally:

```bash
# Android APK
cd mobile
flutter build apk --release

# Android AAB
flutter build appbundle --release

# Web
flutter build web --release

# Windows
flutter build windows --release

# macOS
flutter build macos --release

# Linux
flutter build linux --release

# iOS (macOS only)
flutter build ios --release
```

## Release Process

1. Update version in `mobile/pubspec.yaml`
2. Push changes to the repository
3. Go to Actions → Flutter Release → Run workflow
4. Enter version (e.g., `v1.0.0-rc.1`)
5. Toggle prerelease as needed
6. Wait for build to complete
7. Release will be created automatically with all binaries

## Version Numbering

- **RC**: `v1.0.0-rc.1`, `v1.0.0-rc.2`, ...
- **Beta**: `v1.0.0-beta.1`, `v1.0.0-beta.2`, ...
- **Alpha**: `v1.0.0-alpha.1`, `v1.0.0-alpha.2`, ...
- **Stable**: `v1.0.0`, `v1.0.1`, `v1.1.0`, ...

## Platform Support

| Platform | Build Runner | Notes |
|----------|--------------|-------|
| Android | ubuntu-latest | Requires signing secrets |
| Web | ubuntu-latest | Static files |
| Windows | windows-latest | Requires Windows runner |
| macOS | macos-latest | Requires macOS runner |
| Linux | ubuntu-latest | Requires GTK dependencies |
| iOS | macos-latest | No code signing (for App Store use) |