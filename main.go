// Command bedrock-pack-tools dumps, downloads, decrypts, and encrypts
// Minecraft Bedrock Edition resource packs from servers.
//
// Bedrock servers send AES-256-CFB8 encrypted resource packs whose
// per-pack master keys arrive over the wire during the connection
// handshake. bedrock-pack-tools connects as a Bedrock client (Xbox
// Live device-code auth, token cached locally), captures those keys,
// optionally downloads the packs (handles both the protocol transfer
// and the CDN-URL fallback), and decrypts them offline. It can also
// encrypt a plain resource-pack directory back into a deployable
// .mcpack + .mcpack.key pair.
//
// Usage:
//
//	bedrock-pack-tools keys     <server:port> [output.json]
//	bedrock-pack-tools download <server:port> [output-dir]
//	bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
//	bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
//	bedrock-pack-tools encrypt  <pack-dir> [key] [output.mcpack]
//
// See the README for the full command reference, the on-disk format
// of contents.json, and the keys.json schema produced by 'keys'.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
)

var errUsage = errors.New("usage")

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "keys":
		err = runKeys(os.Args[2:])
	case "download":
		err = runDownload(os.Args[2:])
	case "decrypt":
		err = runDecrypt(os.Args[2:])
	case "encrypt":
		err = runEncrypt(os.Args[2:])
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		if !errors.Is(err, errUsage) {
			fmt.Fprintf(os.Stderr, "\n  %sError: %v%s\n", colorRed, err, colorReset)
		}
		os.Exit(1)
	}
}

func printVersion() {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	fmt.Printf("bedrock-pack-tools %s\n", v)
}

func printUsage() {
	fmt.Println(`bedrock-pack-tools — dump, download, decrypt & encrypt Minecraft Bedrock resource packs

Usage:
  bedrock-pack-tools keys     <server:port> [output.json]
  bedrock-pack-tools download <server:port> [output-dir]
  bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools encrypt  <pack-dir> [key] [output.mcpack]
  bedrock-pack-tools version

Commands:
  keys      Connect to a Bedrock server and extract resource pack encryption keys.
            Requires Xbox Live authentication (device code flow, token cached).

  download  Connect to a Bedrock server, download all resource packs, and extract
            them to disk. Also saves encryption keys. Packs are still encrypted
            on disk — use 'decrypt' afterwards.

  decrypt   Decrypt an encrypted resource pack using a 32-character AES key,
            or batch-decrypt all packs matched by a keys.json file.

  encrypt   Encrypt a plain resource pack into a ready-to-use .mcpack file
            with a .mcpack.key beside it. Uses AES-256-CFB8 with per-file keys.
            If no key is provided, one is generated automatically.

  version   Show version information.

Run "bedrock-pack-tools <command>" with no arguments for command-specific help.`)
}
