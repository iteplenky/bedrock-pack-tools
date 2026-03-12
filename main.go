package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "keys":
		runKeys(os.Args[2:])
	case "decrypt":
		runDecrypt(os.Args[2:])
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
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
	fmt.Println(`bedrock-pack-tools — dump & decrypt Minecraft Bedrock resource packs

Usage:
  bedrock-pack-tools keys    <server:port> [output.json]
  bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]
  bedrock-pack-tools version

Commands:
  keys      Connect to a Bedrock server and extract resource pack encryption keys.
            Requires Xbox Live authentication (device code flow, token cached).

  decrypt   Decrypt an encrypted resource pack using a 32-character AES key,
            or batch-decrypt all packs matched by a keys.json file.

  version   Show version information.

Run "bedrock-pack-tools <command>" with no arguments for command-specific help.`)
}
