# Built-in packages

The build writes the official packages embedded in the Control binary here:
`python3 packages/shared/build_package.py --all --builtin --version <version> --out internal/builtinpkg/dist`
(`make build`, and the release job with the official signing key). For every
package it writes the slim signed artifact without a host binary
(`<id>-<version>.anxp`), the canonical manifest and its signature, and
`index.json` lists them.

Only this file is tracked. Without `index.json` the binary embeds no package
and Control starts without installing any (`docs/architecture/plugin-kernel-contract.md`,
"Built-in Packages").
