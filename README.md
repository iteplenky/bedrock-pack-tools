# Bedrock Pack Tools

[![Release](https://img.shields.io/github/v/release/iteplenky/bedrock-pack-tools?logo=github&sort=semver)](https://github.com/iteplenky/bedrock-pack-tools/releases/latest)
[![Test](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml/badge.svg)](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iteplenky/bedrock-pack-tools/v3.svg)](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3)
[![License: MIT](https://img.shields.io/github/license/iteplenky/bedrock-pack-tools)](LICENSE)

**A Go CLI for working with Minecraft Bedrock resource packs:** dump
encryption keys off a server, download and decrypt them, or re-encrypt
your own. Works against Marketplace partner servers and Mojang's Live
Events catalog.

- **`keys`** - sign in via Xbox Live and pull AES content keys off a server
- **`download`** - grab every pack a server ships in one command
- **`decrypt`** - turn encrypted packs into plain editable directories
- **`encrypt`** - package a plain pack into a deployable `.mcpack` + `.mcpack.key`
- **`featured`** - browse and download from Minecraft's Featured Servers / Live Events catalog

**Scope.** Built for researchers, server operators auditing their own
deployments, and pack authors recovering their own keys. Not for
redistributing someone else's paid content.

## Installation

Pre-built binaries are the fastest path - no Go toolchain needed.

```bash
curl -L https://github.com/iteplenky/bedrock-pack-tools/releases/latest/download/bedrock-pack-tools_darwin_arm64.tar.gz | tar xz
./bedrock-pack-tools --help
```

Archives for `darwin_{amd64,arm64}`, `linux_{amd64,arm64}`,
`windows_{amd64,arm64}` live on the
[Releases](https://github.com/iteplenky/bedrock-pack-tools/releases)
page; each holds one `bedrock-pack-tools` binary, verify with
`checksums.txt`.

**From source** (Go 1.25+): `go install github.com/iteplenky/bedrock-pack-tools/v3@latest`

### Xbox Live sign-in

`keys`, `download`, and `featured` need a Microsoft / Xbox Live
account. On first run you'll see a device code prompt:

```
Auth: no cached token - starting Xbox Live device auth
A URL and code will appear - enter it in your browser.
```

The token is cached locally and reused on subsequent runs. `featured`
also mints an MCToken via PlayFab, cached separately for ~4 hours.

Cache files (`.xbox_token.json`, `.mctoken.json`, `.device_id`) live in
the OS user-config directory:

- macOS: `~/Library/Application Support/bedrock-pack-tools/`
- Linux: `~/.config/bedrock-pack-tools/`
- Windows: `%AppData%\bedrock-pack-tools\`

## Quick start

```bash
./bedrock-pack-tools featured                          # browse what's live
./bedrock-pack-tools download play.example.net:19132   # dump a server's packs
./bedrock-pack-tools decrypt --all play_example_net_19132_keys.json .
```

## Commands

### `keys` - dump encryption keys

```bash
bedrock-pack-tools keys <server:port> [output.json]
```

Connects to a Bedrock server and writes a UUID → key map. Output
defaults to `<sanitized_server>_keys.json` (e.g. `play.example.net:19132`
becomes `play_example_net_19132_keys.json`).

Use this when you already have the encrypted packs on disk and just
need keys; otherwise `download` does both at once.

### `download` - download resource packs

```bash
bedrock-pack-tools download [-v] <server:port> [output-dir]
```

Downloads every pack the server ships, plus the keys file. Each pack
lands in its own `Name_vVersion/` folder. Handles both the
protocol-level pack transfer and the CDN-URL fallback that some
servers use.

`-v` / `--verbose` prints all packet IDs for debugging handshake
issues.

The downloaded packs are still encrypted; the tool prints the exact
`decrypt --all` command to run next.

### `decrypt` - decrypt resource packs

```bash
# Single pack with a known key
bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]

# Batch using the keys.json from `keys` or `download`
bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]
```

Batch mode matches encrypted packs to keys by the UUID in each pack's
`manifest.json`, so the directory layout doesn't matter. The key is the
32-character AES master key; per-file keys live inside the encrypted
`contents.json`.

### `encrypt` - encrypt resource packs

```bash
bedrock-pack-tools encrypt <pack-dir> [key] [output.mcpack]
```

Packages a plain directory into a `.mcpack` plus a `.mcpack.key` beside
it. Each non-manifest file gets its own randomly generated 32-character
AES-256-CFB8 key; `manifest.json` and `pack_icon.png` are copied as-is
(listed in `contents.json` with an empty key, as Bedrock expects).

If no key is supplied, a 32-character alphanumeric one is generated. If
no output path is supplied, the `.mcpack` takes the directory's name.

To deploy: drop the `.mcpack` and `.mcpack.key` into your Bedrock
server's `resource_packs/` directory and register the pack the way your
server normally does.

### `featured` - browse Featured Servers

```bash
bedrock-pack-tools featured                            # list everything
bedrock-pack-tools featured download <index> [output-dir]
```

Lists Mojang's Featured Servers catalog (what the in-game Servers tab
uses) plus any active Live Events, with a live RakNet
ping for each entry. Hostnames come straight from this API, so the
list stays current as partners move.

Each row has five columns - tag, index, name, address, status:

```
  [ON]   [ N]  <server name>         <host>:<port>            online <count> players
  [OFF]  [ N]  <server name>         <host>:<port>            offline
  [EXP]  [ N]  <server name>         (experience-join)        resolve on download
  [EVT]  [ N]  <event name>          (live event)             resolve on download
```

- `[ON]`  - partner with a public `host:port`, online (RakNet ping succeeded)
- `[OFF]` - partner with a public `host:port`, offline (ping failed)
- `[EXP]` - reached via `experienceId`, not a fixed host; resolved on download via `POST /api/v2.0/join/experience`
- `[EVT]` - Mojang Gathering / live event; resolved on download via `/api/v1.0/access` + `/api/v1.0/venue/{gatheringId}`

## File formats

### `keys.json`

Map of pack UUID → encryption info, written by `keys` / `download` and
consumed by `decrypt --all`:

```json
{
  "12345678-1234-1234-1234-123456789abc": {
    "key": "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
    "version": "1.0.0",
    "name": "Example Pack"
  },
  "fedcba98-7654-3210-fedc-ba9876543210": {
    "key": "ZYXWVUTSRQPONMLKJIHGFEDCBA987654",
    "version": "2.3.1",
    "name": "Another Pack"
  }
}
```

### Encrypted pack layout

The `contents.json` file inside an encrypted pack is a 256-byte binary
header followed by AES-256-CFB8 ciphertext.

| Offset    | Bytes | Content                                       |
| --------- | ----- | --------------------------------------------- |
| `0..3`    | 4     | Version (uint32 little-endian, currently `0`) |
| `4..7`    | 4     | Magic bytes `0xFC 0xB9 0xCF 0x9B`             |
| `8..15`   | 8     | Zero padding                                  |
| `16`      | 1     | `'$'` (0x24) separator                        |
| `17..52`  | 36    | Pack UUID, ASCII                              |
| `53..255` | 203   | Zero padding                                  |

After the header comes a JSON document encrypted with the master key
(from the server handshake or the `.mcpack.key` file). The IV is the
first 16 bytes of the 32-byte key.

The decrypted JSON lists every file in the pack with its own per-file
key. `manifest.json` and `pack_icon.png` appear with empty keys and
are stored unencrypted.

## Troubleshooting

**`featured` lists no servers, or different servers than my game.**
Mojang shards the catalog by a PlayFab Experiments cohort, keyed on
`device.id`. The tool persists a stable ID in `.device_id` so your list
stays consistent across runs - and may differ from your in-game client
if you landed in a different cohort. Delete `.device_id` and rerun to
roll a new cohort.

**A `[EVT]` row fails on download.**
Live events come and go. If no venue is active for the gathering at
the moment you try to join, the API returns nothing and the tool
prints a clear message. Wait or pick another entry.

**Handshake hangs against a specific server.**
The tool depends on a patched fork of
[gophertunnel](https://github.com/iteplenky/gophertunnel/tree/fix/deferred-packet-race)
pinned via `replace` in `go.mod` to fix a deferred-packet race that
deadlocks against some servers. If you built from source and stripped
the replace directive, that's why.

**Decryption produces garbage.**
Wrong key, or the encrypted pack was downloaded incompletely (e.g.
truncated CDN response). Re-run `download -v` to confirm the pack
transfer completed.

**Auth keeps re-prompting for the device code.**
The token cache lives in `os.UserConfigDir()` (see [Xbox Live
sign-in](#xbox-live-sign-in) for the per-OS path). If the directory
isn't writable - e.g. running in a sandbox - each run starts auth from
scratch.

## Environment variables

| Variable   | Effect                                                                          |
| ---------- | ------------------------------------------------------------------------------- |
| `NO_COLOR` | Disable all ANSI colors and escape codes ([no-color.org](https://no-color.org/)) |

## Notes

- `.xbox_token.json` and `.mctoken.json` are full auth tokens - do not
  share them.
- Keys are server-specific. Different servers use different keys.
- Packs the server marks unencrypted are listed but excluded from
  `keys.json`.

## License

[MIT](LICENSE)
