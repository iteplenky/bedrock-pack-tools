# Bedrock Pack Tools

[![Release](https://img.shields.io/github/v/release/iteplenky/bedrock-pack-tools?logo=github&sort=semver)](https://github.com/iteplenky/bedrock-pack-tools/releases/latest)
[![Test](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml/badge.svg)](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iteplenky/bedrock-pack-tools.svg)](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools)
[![License: MIT](https://img.shields.io/github/license/iteplenky/bedrock-pack-tools)](LICENSE)

Dump encryption keys, download, decrypt and encrypt Minecraft Bedrock server resource packs.

## How It Works

Many Bedrock servers send encrypted resource packs to clients. Each pack is encrypted
with AES-256-CFB8, and the keys are transmitted during the connection handshake.

This tool connects to a server as a regular Bedrock client (with Xbox Live authentication),
downloads resource packs, captures the encryption keys, and decrypts the packs offline.

### Encryption Format

Encrypted packs contain a `contents.json` file with:
- A **256-byte binary header** (magic bytes + pack UUID)
- **AES-256-CFB8 encrypted JSON** listing every file in the pack and its individual decryption key

The master key (from the server handshake) decrypts `contents.json`. Each file inside
the pack then has its own key listed in the decrypted `contents.json`.

The IV for CFB8 is the first 16 bytes of the 32-byte key.

## Installation

### Pre-built binaries

Download from [Releases](https://github.com/iteplenky/bedrock-pack-tools/releases):

```bash
# macOS / Linux
chmod +x bedrock-pack-tools-*
./bedrock-pack-tools_linux_amd64 --version

# Windows — just run the .exe
```

### From source

Requires **Go 1.25+** ([install](https://go.dev/dl/)):

```bash
git clone https://github.com/iteplenky/bedrock-pack-tools.git
cd bedrock-pack-tools
go build -o bedrock-pack-tools .
```

### Xbox Live account

Required to authenticate with Bedrock servers (for the `keys` and `download` commands).

## Quick Start

```bash
# Download all packs from a server (keys are saved automatically)
bedrock-pack-tools download <server:port> ./packs/

# Decrypt the downloaded packs
bedrock-pack-tools decrypt --all ./packs/server_keys.json ./packs/
```

Or if you only need the keys (e.g. to decrypt packs from a local cache):

```bash
bedrock-pack-tools keys <server:port>
bedrock-pack-tools decrypt --all server_keys.json ./my_packs/
```

## Commands

### `keys` — Dump Encryption Keys

```bash
bedrock-pack-tools keys <server:port> [output.json]
```

Connects to a Bedrock server and extracts resource pack encryption keys.
Output defaults to `<sanitized_server>_keys.json`.

**Authentication:** On first run, you'll see a URL and a code. Open the URL in your
browser and enter the code to sign in with your Microsoft/Xbox account. The token is
cached in `.xbox_token.json` so you only need to authenticate once.

**Examples:**

```bash
# Dump keys, auto-named output
bedrock-pack-tools keys <server:port>

# Dump keys to a specific file
bedrock-pack-tools keys <server:port> my_server_keys.json
```

### `download` — Download Resource Packs

```bash
bedrock-pack-tools download [-v] <server:port> [output-dir]
```

Connects to a Bedrock server, downloads all resource packs, and extracts them to disk.
Also saves encryption keys. Supports both protocol-level pack transfer and CDN fallback.

Output directory will contain one folder per pack: `Name_vVersion/`.

**Flags:**
- `-v, --verbose` — Show all packet IDs for debugging

**Examples:**

```bash
# Download all packs from a server
bedrock-pack-tools download <server:port>

# Download to a specific directory
bedrock-pack-tools download <server:port> ./packs/

# Verbose mode for debugging
bedrock-pack-tools download -v <server:port>
```

### `decrypt` — Decrypt Resource Packs

```bash
# Single pack
bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]

# Batch decrypt using keys.json
bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]
```

**Single pack:** Provide the pack directory and its 32-character AES key.

**Batch mode:** Provide a `keys.json` file (from the `keys` command) and a directory
containing encrypted packs. The tool matches packs by UUID from their `manifest.json`.

**Examples:**

```bash
# Decrypt a single pack
bedrock-pack-tools decrypt ./packs/SomePack_v1.0.0 ABCDEFGHIJKLMNOPQRSTUVWXYZ123456

# Decrypt all packs with keys
bedrock-pack-tools decrypt --all server_keys.json ./packs/
bedrock-pack-tools decrypt --all server_keys.json ./packs/ ./decrypted/
```

### `encrypt` — Encrypt Resource Packs

```bash
bedrock-pack-tools encrypt <pack-dir> [key] [output.mcpack]
```

Encrypts a plain resource pack directory and produces a ready-to-use `.mcpack` file
plus a `.mcpack.key` file beside it. Uses AES-256-CFB8 (the standard Bedrock encryption
format). Each file gets its own randomly generated 32-character key.

`manifest.json` and `pack_icon.png` are copied as-is (listed in `contents.json`
with an empty key, as Bedrock expects).

If no key is provided, a random 32-character alphanumeric key is generated.
If no output path is provided, the `.mcpack` is named after the pack directory.

**Examples:**

```bash
# Encrypt with an auto-generated key → MyPack_v1.0.0.mcpack + MyPack_v1.0.0.mcpack.key
bedrock-pack-tools encrypt ./MyPack_v1.0.0/

# Encrypt with a specific master key
bedrock-pack-tools encrypt ./MyPack_v1.0.0/ ABCDEFGHIJKLMNOPQRSTUVWXYZ123456

# Encrypt to a specific path
bedrock-pack-tools encrypt ./MyPack_v1.0.0/ ABCDEFGHIJKLMNOPQRSTUVWXYZ123456 ./out/MyPack.mcpack
```

To deploy: copy the `.mcpack` and `.mcpack.key` to your Bedrock server's `resource_packs/` directory.

## Keys JSON Format

The `keys` command outputs JSON mapping pack UUIDs to their encryption info:

```json
{
  "12345678-1234-1234-1234-123456789abc": {
    "key": "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
    "version": "1.0.0",
    "name": "Example Pack"
  }
}
```

## Environment Variables

| Variable | Effect |
|---|---|
| `NO_COLOR` | Disable all ANSI colors and escape codes in output ([no-color.org](https://no-color.org/)) |

## License

[MIT](LICENSE)

## Notes

- The `.xbox_token.json` file contains your auth token — do not share it.
- Keys are specific to a server. Different servers use different keys.
- Some servers use CDN URLs for pack delivery instead of the protocol; `download` handles both modes.
- Packs without encryption are listed but not included in the keys output.
- Uses a [patched gophertunnel](https://github.com/iteplenky/gophertunnel/tree/fix/deferred-packet-race) fork to fix a handshake deadlock with certain servers.
- This tool is for personal/educational use. Respect content creators' rights.
