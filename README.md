# bedrock-pack-tools

English | [Русский](README.ru.md)

[![Release](https://img.shields.io/github/v/release/iteplenky/bedrock-pack-tools?logo=github&sort=semver)](https://github.com/iteplenky/bedrock-pack-tools/releases/latest)
[![Test](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml/badge.svg)](https://github.com/iteplenky/bedrock-pack-tools/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iteplenky/bedrock-pack-tools/v3.svg)](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3)
[![License: MIT](https://img.shields.io/github/license/iteplenky/bedrock-pack-tools)](LICENSE)

**Turn a Minecraft Bedrock server's encrypted resource packs into plain, editable folders with a single command.**

bedrock-pack-tools connects to a live Bedrock server the way the game does, pulls the AES content keys the server hands out, downloads every pack, and decrypts them on disk. You don't need a key to begin. It's handy when you run a server and want to audit what it ships, when you wrote a pack and lost the key, or when you're poking at the protocol for research.

```bash
bedrock-pack-tools download --decrypt play.example.net:19132
```

<p align="center">
  <img src="docs/demo.gif" alt="bedrock-pack-tools interactive menu: entering a server address, choosing download and decrypt, and switching the interface language" width="760">
</p>

## Install

Pre-built binaries don't need a Go toolchain. On macOS or Linux, download the build for your machine, unpack it, and run:

```bash
curl -L https://github.com/iteplenky/bedrock-pack-tools/releases/latest/download/bedrock-pack-tools_darwin_arm64.tar.gz | tar xz
./bedrock-pack-tools --help
```

On Windows, grab `bedrock-pack-tools_windows_amd64.zip` from the [latest release](https://github.com/iteplenky/bedrock-pack-tools/releases/latest), unzip it, and run it from PowerShell:

```powershell
.\bedrock-pack-tools.exe --help
```

<details>
<summary>Every platform, and verifying your download</summary>

<br>

| Your machine | Archive |
|---|---|
| macOS, Apple Silicon (M1/M2/M3) | `bedrock-pack-tools_darwin_arm64.tar.gz` |
| macOS, Intel | `bedrock-pack-tools_darwin_amd64.tar.gz` |
| Linux, most PCs and servers | `bedrock-pack-tools_linux_amd64.tar.gz` |
| Linux, ARM (e.g. Raspberry Pi 64-bit) | `bedrock-pack-tools_linux_arm64.tar.gz` |
| Windows, most PCs | `bedrock-pack-tools_windows_amd64.zip` |
| Windows, ARM | `bedrock-pack-tools_windows_arm64.zip` |

Every release ships a `checksums.txt`, so you can verify what you downloaded with `sha256sum -c checksums.txt`.

</details>

Rather build it yourself? With Go 1.25 or newer:

```bash
go install github.com/iteplenky/bedrock-pack-tools/v3@latest
```

Keep the `/v3` (it's part of the module path). The binary lands in `$(go env GOPATH)/bin`; add that to your `PATH` if it isn't already.

## Usage

The command up top is the whole point: aim it at a server and it pulls every pack and decrypts it in one go. The first run signs you in with Xbox Live, which is a one-time step. It prints a short code, you paste it into the page it opens, and the token is cached after that. When it finishes you'll have a `play.example.net/` folder with the decrypted packs under `decrypted/` and a `keys.json` mapping each pack's UUID to its key.

Run it with no arguments and you get an interactive menu, which is the gentler way in:

```bash
bedrock-pack-tools
```

From there you can browse the in-game featured servers, type any address and pick what to do with it, re-decrypt something you already downloaded, package a pack back up, or flip the interface between English and Russian. Everything reports progress as it works; `p` pauses, `esc` backs out.

## Commands

| Command | What it does | Example |
| --- | --- | --- |
| `keys` | Pull a server's AES content keys into a `keys.json` | `bedrock-pack-tools keys play.example.net:19132` |
| `download` | Download every pack a server ships, plus the keys | `bedrock-pack-tools download --decrypt play.example.net:19132` |
| `decrypt` | Turn an encrypted pack into a plain, editable folder | `bedrock-pack-tools decrypt --all keys.json packs/` |
| `encrypt` | Package a plain pack into a `.mcpack` + `.mcpack.key` | `bedrock-pack-tools encrypt my-pack/` |
| `featured` | Browse and download from the in-game Featured Servers / Live Events catalog | `bedrock-pack-tools featured` |
| `login` | Sign in with Xbox Live and cache the token | `bedrock-pack-tools login` |
| `logout` | Remove the cached Xbox and MCToken files | `bedrock-pack-tools logout` |
| `version` | Print the build version | `bedrock-pack-tools version` |

A few details the table can't hold:

- `download` writes each server into its own `<server>/` folder (the packs, a `keys.json`, and a `decrypted/` subfolder when you pass `--decrypt`), so dumps from different servers never collide on a shared pack name. Skip `--decrypt` and it prints the `decrypt --all` command to run later instead. Pass `-v` for a verbose transfer log.
- `decrypt --all <keys.json> <packs-dir>` matches packs to keys by the UUID in each pack's `manifest.json`, so the folder layout doesn't matter.
- `encrypt` makes a fresh 32-character AES-256-CFB8 key per file. Pass your own instead, or use `--key-out PATH` to write the master key outside the pack folder.
- `featured` tags each row `[ON]` / `[OFF]` (a public `host:port` it could RakNet-ping), `[EXP]` (resolved from an `experienceId` at download time), or `[EVT]` (a live event, also resolved on download). Grab one with `featured download <index>`.

Run `bedrock-pack-tools <command> --help` for the full syntax of any command, or read the [godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3) for the internals.

<details>
<summary>The <code>keys.json</code> file</summary>

<br>

`keys` and `download` write a map of pack UUID to its encryption info, which `decrypt --all` reads back:

```json
{
  "12345678-1234-1234-1234-123456789abc": {
    "key": "ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
    "version": "1.0.0",
    "name": "Example Pack"
  }
}
```

The per-file keys live inside each encrypted pack's `contents.json`; the tool reads them automatically once it has the master key. The [godoc](https://pkg.go.dev/github.com/iteplenky/bedrock-pack-tools/v3) documents the encrypted-pack binary layout.

</details>

### Signing in

`keys`, `download`, and `featured` talk to Xbox Live, so they need a Microsoft account. The first time, you'll see something like:

```
Auth: no cached token - starting Xbox Live device auth
A URL and code will appear - enter it in your browser.
```

After that the token is reused. `featured` also caches a separate PlayFab token for about four hours. All of it sits in your OS config directory:

- macOS: `~/Library/Application Support/bedrock-pack-tools/`
- Linux: `~/.config/bedrock-pack-tools/`
- Windows: `%AppData%\bedrock-pack-tools\`

## FAQ

**How do I open a `.mcpack`?**
It's a zip archive, so rename it to `.zip` and extract. If the files inside look scrambled (the `contents.json` starts with a binary header), decrypt the pack first.

**What's the 32-character key?**
It's the pack's AES-256 master key. The IV is the first 16 bytes of it, and the per-file keys sit inside the encrypted `contents.json`.

**Is this allowed?**
Use it on servers you run or have permission to test, and packs you have a right to. See [Responsible use](#responsible-use).

<details>
<summary>Troubleshooting</summary>

<br>

- **`featured` is empty, or shows different servers than the game does.** Mojang shards the catalog by a PlayFab cohort tied to a stable `.device_id`. Use **Settings > Reset featured cohort** (or delete `.device_id`) to roll a new one.
- **An `[EVT]` row fails to download.** No venue is live for that gathering right now; the tool says so. Wait, or pick another entry.
- **Decryption gives you garbage.** Either the key is wrong or the pack didn't download fully. Re-run with `download -v` to confirm the transfer completed.
- **It keeps asking for the device code.** The token cache directory (see [Signing in](#signing-in)) isn't writable, which is common inside sandboxes, so every run starts auth from scratch.

</details>

## Environment variables

| Variable | Effect |
| --- | --- |
| `NO_COLOR` | Turn off all ANSI colors ([no-color.org](https://no-color.org/)) |
| `BPT_DIAL_TIMEOUT` | Override the `keys` / `download` dial timeout as a Go duration (`5m`, `90s`). It bounds the whole run, CDN fetches included. |
| `BPT_LANG` | Force the interface language (`en` or `ru`); otherwise it follows your locale. You can also pass `--lang` / `-lang`. |

## Responsible use

This is for people auditing their own servers, recovering their own keys, or doing legitimate research. Use it on servers you run or are authorized to test, with packs you have the right to access, and not to redistribute paid or third-party content. You're responsible for following the law and the terms of whatever you connect to (see [LICENSE](LICENSE)).

The cached tokens (`.xbox_token.json`, `.mctoken.json`) and any `keys.json` you produce are real credentials: the tokens reach your account, and the keys decrypt content. Keep your config directory private, and don't commit or share what's in it.

## Contributing

Issues and pull requests are welcome; please open an issue first for anything non-trivial. Run `go build ./... && go test ./...` before sending, and keep commit subjects conventional (`feat:`, `fix:`, `docs:`, ...). [CONTRIBUTING.md](CONTRIBUTING.md) has the full checklist.

## License

MIT. Do what you like with it, just keep the notice. See [LICENSE](LICENSE).
