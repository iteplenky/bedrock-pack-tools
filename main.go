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
//	bedrock-pack-tools                                  (no command: interactive menu)
//	bedrock-pack-tools keys     <server:port> [output.json]
//	bedrock-pack-tools download [--decrypt] <server:port> [output-dir]
//	bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
//	bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
//	bedrock-pack-tools encrypt  <pack-dir> [key] [output.mcpack]
//	bedrock-pack-tools featured [download <index> [output-dir]]
//	bedrock-pack-tools version
//
// See the README for the full command reference, the on-disk format
// of contents.json, and the keys.json schema produced by 'keys'.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"
)

var errUsage = errors.New("usage")

// errPartialResult signals "produced useful output but not fully
// done" - maps to exit 2 in main so CI can distinguish from success.
// The command itself prints what landed.
var errPartialResult = errors.New("partial result")

var version = "dev"

const dialTimeoutEnv = "BPT_DIAL_TIMEOUT"

// dialTimeout returns the DialContext timeout for keys/download.
// Honours BPT_DIAL_TIMEOUT as a Go duration ("5m", "90s") when valid;
// falls back to the caller default otherwise.
func dialTimeout(fallback time.Duration) time.Duration {
	if v := os.Getenv(dialTimeoutEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		fmt.Fprintf(os.Stderr, "  Warning: %s=%q is not a valid duration, using %s\n", dialTimeoutEnv, v, fallback)
	}
	return fallback
}

func main() {
	if len(os.Args) < 2 {
		// No command on a real terminal: open the interactive menu.
		if isInteractive() {
			if err := runTUI(); err != nil {
				os.Exit(handleErr(os.Stderr, err))
			}
			return
		}
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
	case "featured":
		err = runFeatured(os.Args[2:])
	case "login":
		err = runLogin(os.Args[2:])
	case "logout":
		err = runLogout(os.Args[2:])
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
		os.Exit(handleErr(os.Stderr, err))
	}
}

// handleErr renders err and returns the exit code. Pulled out of main
// so the classification is testable without os.Exit.
func handleErr(w io.Writer, err error) int {
	switch {
	case errors.Is(err, context.Canceled):
		return 130 // SIGINT convention
	case errors.Is(err, errUsage):
		return 1 // usage already printed
	case errors.Is(err, errPartialResult):
		return 2 // summary already printed
	}
	if d, ok := humanize(err); ok {
		writeDiagnostic(w, d, err)
	} else {
		writeRawError(w, err)
	}
	return 1
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
	fmt.Println(`bedrock-pack-tools - dump, download, decrypt & encrypt Minecraft Bedrock resource packs

Usage:
  bedrock-pack-tools                                  (no command: interactive menu)
  bedrock-pack-tools keys     <server:port> [output.json]
  bedrock-pack-tools download [--decrypt] <server:port> [output-dir]
  bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools encrypt  [--key-out PATH] <pack-dir> [key] [output.mcpack]
  bedrock-pack-tools featured [download <index> [output-dir]]
  bedrock-pack-tools login
  bedrock-pack-tools logout
  bedrock-pack-tools version

Environment:
  BPT_DIAL_TIMEOUT  Override the keys/download dial timeout (e.g. "5m", "90s").
                    Useful for slow servers or long ResourcePacksInfo waits.

Commands:
  keys      Connect to a Bedrock server and extract resource pack encryption keys.
            Requires Xbox Live authentication (device code flow, token cached).

  download  Connect to a Bedrock server, download all resource packs, and extract
            them to disk. Also saves encryption keys. Packs are encrypted on disk;
            add --decrypt to decrypt in the same step, or run 'decrypt' afterwards.

  decrypt   Decrypt an encrypted resource pack using a 32-character AES key,
            or batch-decrypt all packs matched by a keys.json file.

  encrypt   Encrypt a plain resource pack into a ready-to-use .mcpack file
            with a .mcpack.key beside it. Uses AES-256-CFB8 with per-file keys.
            If no key is provided, one is generated automatically.

  featured  List the Featured Servers and Live Events from Minecraft's
            client-discovery API and optionally download one by index.

  login     Sign in via Xbox Live (device code flow) and cache the token.
  logout    Remove the cached Xbox + franchise tokens.

  version   Show version information.

Run "bedrock-pack-tools <command>" with no arguments for command-specific help.`)
}
