# shellman

Global installer package for Shellman.

## Install

Stable channel:

```bash
npm install -g shellman
```

Dev channel:

```bash
npm install -g shellman@dev
```

## How It Works

- `postinstall` reads `release.json` from GitHub Releases.
- It picks the matching artifact for the current platform and verifies SHA-256.
- Supported targets:
  - darwin-arm64
  - darwin-amd64
  - linux-arm64
  - linux-amd64

At runtime, `shellman` checks remote `release.json` periodically and prints an upgrade hint when a newer channel version is available.

## Environment Variables

- `SHELLMAN_RELEASE_CHANNEL=stable|dev`
- `SHELLMAN_GITHUB_REPO=owner/repo`
- `SHELLMAN_RELEASE_MANIFEST_URL=https://.../release.json`
- `SHELLMAN_SKIP_POSTINSTALL=1`
- `SHELLMAN_FORCE_POSTINSTALL=1`
- `SHELLMAN_NO_UPDATE_CHECK=1`
- `SHELLMAN_UPDATE_CHECK_INTERVAL_SEC=43200`
