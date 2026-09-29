# AnixOps Mobile App - Android Build

This directory contains Android-specific configuration for the AnixOps mobile app.

## Requirements

- Flutter SDK 3.16+
- Android SDK 21+ (minSdk)
- Android SDK 34 (targetSdk)
- Java 17
- Gradle 8.0

## Setup

1. Copy `local.properties.example` to `local.properties` and fill in your paths:
   ```bash
   cp local.properties.example local.properties
   ```

2. Copy `key.properties.example` to `key.properties` and configure your signing:
   ```bash
   cp key.properties.example key.properties
   ```

3. Copy `google-services.json.example` to `google-services.json` and add your Firebase config:
   ```bash
   cp google-services.json.example google-services.json
   ```

## Signing

For release builds, you need to create a keystore:

```bash
keytool -genkey -v -keystore ../keystore/anixops-release.jks -keyalg RSA -keysize 2048 -validity 10000 -alias anixops
```

## Build

```bash
# Debug APK
flutter build apk --debug

# Release APK
flutter build apk --release

# App Bundle (for Play Store)
flutter build appbundle --release
```

## Google Play Store Requirements

Before uploading to Google Play:

1. ✅ All icons provided (mipmap folders)
2. ✅ Privacy Policy page included
3. ✅ Terms of Service page included
4. ✅ Network Security Config
5. ✅ ProGuard rules configured
6. ✅ Signed with release key
7. ✅ App Bundle format (.aab)
8. ⬜ Content rating questionnaire
9. ⬜ Store listing (screenshots, descriptions)

## Directory Structure

```
android/
├── app/
│   ├── src/main/
│   │   ├── kotlin/          # Kotlin source
│   │   ├── res/             # Android resources
│   │   │   ├── drawable/    # Icons and graphics
│   │   │   ├── mipmap*/     # App icons
│   │   │   ├── values/      # Strings and colors
│   │   │   └── xml/         # Config files
│   │   └── AndroidManifest.xml
│   ├── build.gradle         # App-level build config
│   └── google-services.json # Firebase config (git-ignored)
├── gradle/wrapper/          # Gradle wrapper
├── build.gradle             # Project-level build config
├── settings.gradle          # Project settings
├── gradle.properties        # Gradle properties
├── key.properties           # Signing config (git-ignored)
└── local.properties         # SDK paths (git-ignored)
```