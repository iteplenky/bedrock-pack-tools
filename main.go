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
	fmt.Println(`bedrock-pack-tools — dump, download & decrypt Minecraft Bedrock resource packs

Usage:
  bedrock-pack-tools keys     <server:port> [output.json]
  bedrock-pack-tools download <server:port> [output-dir]
  bedrock-pack-tools decrypt  <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt  --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools version

Commands:
  keys      Connect to a Bedrock server and extract resource pack encryption keys.
            Requires Xbox Live authentication (device code flow, token cached).

  download  Connect to a Bedrock server, download all resource packs, and extract
            them to disk. Also saves encryption keys. Packs are still encrypted
            on disk — use 'decrypt' afterwards.

  decrypt   Decrypt an encrypted resource pack using a 32-character AES key,
            or batch-decrypt all packs matched by a keys.json file.

  version   Show version information.

Run "bedrock-pack-tools <command>" with no arguments for command-specific help.`)
}
