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
//	bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]
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

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"

	// Side-effect import: the message catalogs register their EN/RU
	// entries with lang via init() before main runs.
	_ "github.com/iteplenky/bedrock-pack-tools/v3/internal/messages"
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
		fmt.Fprint(os.Stderr, lang.Tf("usage.dialtimeout.warning", dialTimeoutEnv, v, fallback))
	}
	return fallback
}

func main() {
	// Resolve --lang / -lang first - it's a global that applies to every
	// subcommand and the no-arg menu, so strip it from the args before
	// dispatch and fix the active language before any user-facing output.
	langValue, args := extractGlobalLang(os.Args[1:])
	lang.Init(langValue, loadStore().Language)

	if len(args) < 1 {
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
	switch args[0] {
	case "keys":
		err = runKeys(args[1:])
	case "download":
		err = runDownload(args[1:])
	case "decrypt":
		err = runDecrypt(args[1:])
	case "encrypt":
		err = runEncrypt(args[1:])
	case "featured":
		err = runFeatured(args[1:])
	case "login":
		err = runLogin(args[1:])
	case "logout":
		err = runLogout(args[1:])
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprint(os.Stderr, lang.Tf("usage.unknown.command", args[0]))
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
	fmt.Println(lang.T("usage.help"))
}
