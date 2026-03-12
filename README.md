# Bedrock Pack Tools

Dump encryption keys from Minecraft Bedrock servers and decrypt their resource packs.

## How It Works

Many Bedrock servers send encrypted resource packs to clients. Each pack is encrypted
with AES-256-CFB8, and the keys are transmitted during the connection handshake.

This tool connects to a server as a regular Bedrock client (with Xbox Live authentication),
captures the encryption keys, and can then decrypt the downloaded packs offline.

### Encryption Format

Encrypted packs contain a `contents.json` file with:
- A **256-byte binary header** (magic bytes + pack UUID)
- **AES-256-CFB8 encrypted JSON** listing every file in the pack and its individual decryption key

The master key (from the server handshake) decrypts `contents.json`. Each file inside
the pack then has its own key listed in the decrypted `contents.json`.

The IV for CFB8 is the first 16 bytes of the 32-byte key.

## Installation

### Pre-built binaries

Download from [Releases](https://github.com/iteplenky/bedrock-pack-tools/releases) and make executable:

```bash
chmod +x bedrock-pack-tools-*
```

### From source

Requires **Go 1.25+** ([install](https://go.dev/dl/)):

```bash
go install github.com/iteplenky/bedrock-pack-tools@latest
```

### Xbox Live account

Required to authenticate with Bedrock servers (for the `keys` command).

## Quick Start

```bash
# 1. Dump keys from a server
bedrock-pack-tools keys play.example.net:19132

# 2. Decrypt packs using the dumped keys
bedrock-pack-tools decrypt --all play_example_net_19132_keys.json ./my_packs/
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
bedrock-pack-tools keys play.example.net:19132

# Dump keys to a specific file
bedrock-pack-tools keys play.example.net:19132 my_server_keys.json
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

## Building from Source

```bash
git clone https://github.com/iteplenky/bedrock-pack-tools.git
cd bedrock-pack-tools
go build -o bedrock-pack-tools .
```

## License

[MIT](LICENSE)

## Notes

- The `.xbox_token.json` file contains your auth token — do not share it.
- Keys are specific to a server. Different servers use different keys.
- Packs without encryption are listed but not included in the keys output.
- This tool is for personal/educational use. Respect content creators' rights.
