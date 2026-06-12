# bedrock-pack-tools

English | [Русский](README.ru.md)

[![Release](https://img.shields.io/github/v/release/iteplenky/bedrock-pack-tools?logo=github&sort=semver)](https://github.com/iteplenky/bedrock-pack-tools/releases/latest)
[![Test](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml/badge.svg)](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iteplenky/bedrock-pack-tools/v3.svg)](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3)
[![License: MIT](https://img.shields.io/github/license/iteplenky/bedrock-pack-tools)](LICENSE)

**Dump AES content keys, download, and decrypt Minecraft Bedrock encrypted
resource packs from the command line.** A cross-platform Go CLI for security
researchers, server operators auditing their own deployments, and pack authors
recovering their own keys - pull the keys straight off a live Bedrock server
over Xbox Live, no key needed up front, then turn encrypted `.mcpack` files
into plain editable folders or re-encrypt your own.

- **`keys`** - sign in via Xbox Live and pull a server's AES content keys to a `keys.json`, no key needed up front
- **`download`** - download every resource pack a Bedrock server ships, plus the keys, in one command (add `--decrypt` for ready-to-use folders)
- **`decrypt`** - turn an encrypted `.mcpack` into a plain, editable directory
- **`encrypt`** - package a plain resource pack into a deployable `.mcpack` + `.mcpack.key`
- **`featured`** - browse and download from Minecraft's Featured Servers / Live Events catalog
- **`login`** / **`logout`** - sign in via Xbox Live (device code flow) or clear the cached tokens
- **`version`** - print the build version
- **interactive menu** - run with no command for a sectioned menu: browse Featured Servers, enter any `host:port` and pick an action, decrypt past downloads, encrypt a local pack, or open Settings - watching live progress (`p` pauses, `esc` cancels)

**Scope.** Built for researchers, server operators auditing their own
deployments, and pack authors recovering their own keys. Not for
redistributing someone else's paid content.

## What it does in one command

```bash
# Download every encrypted pack a server ships, then decrypt them, in one step:
bedrock-pack-tools download --decrypt play.example.net:19132
```

You get plain, editable pack folders plus a `keys.json` (UUID to AES key) -
ready to inspect, diff, or re-pack. Run with no arguments for an interactive
menu instead.

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Commands](#commands)
- [FAQ](#faq)
- [Troubleshooting](#troubleshooting)
- [Environment variables](#environment-variables)
- [Security](#security)
- [Contributing](#contributing)
- [License](#license)

## Installation

### Pre-built binary (fastest, no Go toolchain)

Grab the archive for your machine from the
[Releases page](https://github.com/iteplenky/bedrock-pack-tools/releases/latest):

| Your machine | Archive |
|---|---|
| macOS, Apple Silicon (M1/M2/M3) | `bedrock-pack-tools_darwin_arm64.tar.gz` |
| macOS, Intel | `bedrock-pack-tools_darwin_amd64.tar.gz` |
| Linux, most PCs and servers | `bedrock-pack-tools_linux_amd64.tar.gz` |
| Linux, ARM (e.g. Raspberry Pi 64-bit) | `bedrock-pack-tools_linux_arm64.tar.gz` |
| Windows, most PCs | `bedrock-pack-tools_windows_amd64.zip` |
| Windows, ARM | `bedrock-pack-tools_windows_arm64.zip` |

Download and unpack (swap in your archive name):

```bash
curl -L https://github.com/iteplenky/bedrock-pack-tools/releases/latest/download/bedrock-pack-tools_darwin_arm64.tar.gz | tar xz
./bedrock-pack-tools --help
```

Windows: extract the `.zip` and run `bedrock-pack-tools.exe --help`. Verify any
download against `checksums.txt` from the Releases page (`sha256sum -c checksums.txt`).

### From source (Go 1.25+)

```bash
go install github.com/iteplenky/bedrock-pack-tools/v3@latest
```

The `/v3` suffix is required. The binary lands in `$(go env GOPATH)/bin` - add it to your `PATH` if needed.

### Xbox Live sign-in

`keys`, `download`, and `featured` need a Microsoft / Xbox Live account. On
first run you'll see a device code prompt:

```
Auth: no cached token - starting Xbox Live device auth
A URL and code will appear - enter it in your browser.
```

The token is cached locally and reused on subsequent runs. `featured` also mints
an MCToken via PlayFab, cached separately for ~4 hours. Cache files
(`.xbox_token.json`, `.mctoken.json`, `.device_id`) live in the OS user-config
directory:

- macOS: `~/Library/Application Support/bedrock-pack-tools/`
- Linux: `~/.config/bedrock-pack-tools/`
- Windows: `%AppData%\bedrock-pack-tools\`

## Quick start

Run with no arguments for the interactive menu - pick a Featured server or enter
any `host:port`, choose an action (download, download + decrypt, or keys only),
and watch progress in place (`p` pauses, `esc` cancels). The menu also covers
**Decrypt packs**, **Encrypt a pack**, and **Settings** (sign in/out, clear
saved addresses or download history, reset the featured cohort):

```bash
./bedrock-pack-tools
```

From the command line:

```bash
./bedrock-pack-tools featured                                   # browse what's live
./bedrock-pack-tools download --decrypt play.example.net:19132  # dump + decrypt in one step
```

## Commands

| Command | Purpose | Example |
| --- | --- | --- |
| `keys` | Pull a server's AES content keys to a `keys.json` | `bedrock-pack-tools keys play.example.net:19132` |
| `download` | Download every pack a server ships, plus the keys | `bedrock-pack-tools download --decrypt play.example.net:19132` |
| `decrypt` | Turn an encrypted pack into a plain editable folder | `bedrock-pack-tools decrypt --all keys.json packs/` |
| `encrypt` | Package a plain pack into a `.mcpack` + `.mcpack.key` | `bedrock-pack-tools encrypt my-pack/` |
| `featured` | Browse and download from the Featured Servers / Live Events catalog | `bedrock-pack-tools featured` |
| `login` | Sign in via Xbox Live and cache the token | `bedrock-pack-tools login` |
| `logout` | Remove the cached Xbox + MCToken auth files | `bedrock-pack-tools logout` |
| `version` | Print the build version | `bedrock-pack-tools version` |

A few flags the table can't carry:

- `download` puts each server's output in its own `<server>/` folder (the packs,
  a `keys.json`, and a `decrypted/` subfolder when you pass `--decrypt`), so dumps
  from multiple servers never pile up or collide on a shared pack name. `--decrypt`
  decrypts every pack in the same step; without it the tool prints the
  `decrypt --all` command to run next.
- `decrypt --all <keys.json> <packs-dir>` batch-decrypts, matching packs to keys
  by the UUID in each pack's `manifest.json`, so the directory layout doesn't matter.
- `encrypt` generates a fresh 32-character AES-256-CFB8 key per file; pass your
  own key, or `--key-out PATH` to write the master key outside the pack directory.
- `featured` rows are tagged `[ON]`/`[OFF]` (public `host:port`, RakNet ping
  result), `[EXP]` (resolved by `experienceId` on download), or `[EVT]` (live
  event, resolved on download).

Run `bedrock-pack-tools <command> --help` for full syntax and options, or see
the [godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3) for internals.

### `keys.json`

Map of pack UUID -> encryption info, written by `keys` / `download` and consumed
by `decrypt --all`:

```json
{
  "12345678-1234-1234-1234-123456789abc": {
    "key": "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
    "version": "1.0.0",
    "name": "Example Pack"
  }
}
```

Per-file keys live inside each encrypted pack's `contents.json`; the tool reads
them automatically once it has the master key. See the
[godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3) for the
encrypted-pack binary layout.

## FAQ

**How do I convert a `.mcpack` to a `.zip`?**
A `.mcpack` is a zip archive - rename the extension to `.zip` and extract it. If
the contents are encrypted (a binary `contents.json` header), run `decrypt` with
the matching key first.

**What is the 32-character key?**
It's the AES-256 master key for the pack. The IV is the first 16 bytes of the
key, and the per-file keys live inside the encrypted `contents.json`.

**Is this allowed?**
It's built for security researchers, server operators auditing their own
deployments, and pack authors recovering their own keys. Don't use it to
redistribute someone else's paid content.

## Troubleshooting

- **`featured` lists no servers, or different servers than your game.** Mojang
  shards the catalog by a PlayFab cohort keyed on a stable `.device_id`. Use
  **Settings > Reset featured cohort** (or delete `.device_id`) to roll a new one.
- **An `[EVT]` row fails on download.** No venue is active for that gathering
  right now; the tool prints a clear message. Wait or pick another entry.
- **Decryption produces garbage.** Wrong key, or the pack downloaded
  incompletely. Re-run `download -v` to confirm the transfer completed.
- **Auth keeps re-prompting for the device code.** The token cache directory
  (see [Xbox Live sign-in](#xbox-live-sign-in)) isn't writable - common in
  sandboxes - so each run starts auth from scratch.

## Environment variables

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | Disable all ANSI colors ([no-color.org](https://no-color.org/)) |
| `BPT_DIAL_TIMEOUT` | Override the `keys` / `download` dial timeout, as a Go duration (`5m`, `90s`). Bounds the whole run, including CDN fetches. |

## Security

The tool authenticates as a normal Minecraft Bedrock client, so you need a
Microsoft / Xbox Live account. The cached tokens (`.xbox_token.json`,
`.mctoken.json`) and any `keys.json` you produce are real credentials - the
tokens grant access to your account, and the keys decrypt paid content.

- Treat the OS user-config directory (see [Xbox Live sign-in](#xbox-live-sign-in)
  for the per-OS path) as secret; don't commit or share its contents.
- Use this on deployments you run and packs you own. Audit your own servers,
  recover your own keys - not to redistribute someone else's paid content.

## Contributing

Issues and pull requests are welcome - please open an issue first to discuss
anything non-trivial. Run `go build ./... && go test ./...` before submitting,
and use conventional-commit subjects (`feat:`, `fix:`, `docs:`, ...). See
[CONTRIBUTING.md](CONTRIBUTING.md) for the full gate.

## License

Released under the MIT License (SPDX: `MIT`). See [LICENSE](LICENSE).
