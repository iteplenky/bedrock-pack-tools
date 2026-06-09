# bedrock-pack-tools

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
- **`login`** - sign in via Xbox Live (device code flow) and cache the token
- **`logout`** - remove the cached Xbox + MCToken auth files
- **`version`** - print the build version
- **interactive menu** - run with no command for a sectioned menu: browse the Featured Servers (filter as you type, multi-select with space) or enter any `host:port`, pick an action (download, download + decrypt, or keys only), and watch live progress - pause with `p`, cancel with `esc`

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

The easiest way is the interactive menu - run with no arguments, pick a
server (or type an address), choose what to do, and watch it run without
leaving the menu:

```bash
./bedrock-pack-tools
```

Pick **Featured servers** to browse the live catalog (filter as you type,
`space` to multi-select) or **Enter a server address** to point at any
`host:port`. Then choose an action - download, download + decrypt, or keys
only - and it streams progress in place: `p` pauses, `esc` cancels, and you
land back on the menu when it finishes.

Or from the command line:

```bash
./bedrock-pack-tools featured                                   # browse what's live
./bedrock-pack-tools download --decrypt play.example.net:19132  # dump + decrypt in one step
```

## Commands

### `keys` - dump a Bedrock server's encryption keys

```bash
bedrock-pack-tools keys <server:port> [output.json]
```

Connects to a Bedrock server and writes a UUID → key map. Output
defaults to `<sanitized_server>_keys.json` (e.g. `play.example.net:19132`
becomes `play_example_net_19132_keys.json`).

Use this when you already have the encrypted packs on disk and just
need keys; otherwise `download` does both at once.

### `download` - download resource packs from a Bedrock server

```bash
bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]
```

Downloads every pack the server ships, plus the keys file. Each pack
lands in its own `Name_vVersion/` folder. Handles both the
protocol-level pack transfer and the CDN-URL fallback that some
servers use.

`-d` / `--decrypt` decrypts the packs right after downloading, so you
get ready-to-use folders in one step. The decrypted packs land in
`decrypted/<server>/` (grouped by server so multiple dumps don't mix);
the command prints the exact location when it finishes.

`-v` / `--verbose` prints all packet IDs for debugging handshake
issues.

Without `--decrypt` the downloaded packs are still encrypted; the tool
prints the exact `decrypt --all` command to run next.

### `decrypt` - decrypt encrypted resource packs

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

### `encrypt` - encrypt a resource pack into a `.mcpack`

```bash
bedrock-pack-tools encrypt [--key-out PATH] <pack-dir> [key] [output.mcpack]
```

Packages a plain directory into a `.mcpack` plus a `.mcpack.key` beside
it. Each non-manifest file gets its own randomly generated 32-character
AES-256-CFB8 key; `manifest.json` and `pack_icon.png` are copied as-is
(listed in `contents.json` with an empty key, as Bedrock expects).

If no key is supplied, a 32-character alphanumeric one is generated. If
no output path is supplied, the `.mcpack` takes the directory's name.

`--key-out PATH` (`-k PATH`) writes the master key to a path you choose
instead of the default `<output.mcpack>.key` beside the pack - handy
for keeping keys out of the directory you hand off.

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

### `login` - sign in via Xbox Live

```bash
bedrock-pack-tools login
```

Runs the Xbox Live device-code flow if no token is cached, then caches it. Use
it to pre-authenticate before running `keys`, `download`, or `featured` (or the
interactive menu) instead of being prompted mid-action.

### `logout` - clear the cached tokens

```bash
bedrock-pack-tools logout
```

Removes the cached Xbox token and MCToken so the next authenticated command
re-prompts for sign-in. The `.device_id` cohort is left in place.

### `version` - print the build version

```bash
bedrock-pack-tools version   # also -v / --version
```

Prints the goreleaser-stamped tag for a released binary, or the module
version (via `debug.ReadBuildInfo`) when built from source.

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

## FAQ

**How do I decrypt a Minecraft Bedrock resource pack?**
If you already have the 32-character key, run `decrypt <pack-dir> <key>`. If you
have a `keys.json` from `keys` or `download`, run `decrypt --all <keys.json>
<packs-dir>` to batch-decrypt every pack at once - it matches packs to keys by
the UUID in each `manifest.json`. Decrypt only packs you own or are authorized
to audit.

**How do I get the encryption key for a Bedrock resource pack?**
The `keys` command signs in via Xbox Live and pulls the AES content keys
straight off a server you run or are authorized to audit, so you don't need the
key ahead of time. The output is a UUID-to-key map you can feed to
`decrypt --all`.

**How do I download every resource pack from a Bedrock server?**
Run `download <server:port>` to fetch every pack the server ships plus its keys
file in one command. Add `--decrypt` to get ready-to-use, decrypted folders in
the same step.

**How do I convert a `.mcpack` to a `.zip`?**
A `.mcpack` is a zip archive - rename the extension to `.zip` and extract it. If
the contents are encrypted (a binary `contents.json` header), run `decrypt` with
the matching key first to get plain files.

**What is the 32-character key, and where do the per-file keys live?**
It's the AES-256 master key for the pack. The per-file keys live inside the
pack's encrypted `contents.json`; the tool reads them automatically once it has
the master key. The IV is the first 16 bytes of the 32-byte key.

**Can I re-encrypt my own pack into a `.mcpack`?**
Yes. `encrypt <pack-dir>` packages a plain directory into a `.mcpack` plus a
`.mcpack.key`, generating a fresh 32-character AES-256-CFB8 key per file. Drop
both into your server's `resource_packs/` directory to deploy.

**Is this allowed?**
It's built for security researchers, server operators auditing their own
deployments, and pack authors recovering their own keys. Don't use it to
redistribute someone else's paid content.

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
The tool requires a patched fork of
[gophertunnel](https://github.com/iteplenky/gophertunnel/tree/fix/deferred-packet-race),
pinned in `go.mod` (the `github.com/iteplenky/gophertunnel` require line), to
fix a deferred-packet race that deadlocks against some servers. If you built
from source and swapped that require back to upstream gophertunnel, that's why.

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

| Variable           | Effect                                                                          |
| ------------------ | ------------------------------------------------------------------------------- |
| `NO_COLOR`         | Disable all ANSI colors and escape codes ([no-color.org](https://no-color.org/)) |
| `BPT_DIAL_TIMEOUT` | Override the `keys` / `download` dial timeout, as a Go duration (`5m`, `90s`). Useful for slow servers or long pack-info waits. Bounds the whole keys / download run - the initial dial and any CDN fetches share this deadline. |

## Notes

- `.xbox_token.json` and `.mctoken.json` are full auth tokens - do not
  share them.
- Keys are server-specific. Different servers use different keys.
- Packs the server marks unencrypted are listed but excluded from
  `keys.json`.

## License

[MIT](LICENSE)
