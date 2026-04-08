package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  binary-cache       Unpack DNAR into a local binary cache\n")
		fmt.Fprintf(os.Stderr, "  nix-store-export   Convert DNAR to nix-store --export format\n")
		fmt.Fprintf(os.Stderr, "\nRun '%s <command> -help' for command-specific help.\n", os.Args[0])
	}

	bcCmd := flag.NewFlagSet("binary-cache", flag.ExitOnError)
	bcCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: binary-cache [options]\n\n")
		fmt.Fprintf(os.Stderr, "Unpack DNAR into a local binary cache.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		bcCmd.PrintDefaults()
	}

	var bcDir string
	bcCmd.StringVar(&bcDir, "cache", "", "binary cache directory")

	var bcInput string
	bcCmd.StringVar(&bcInput, "input", "delta.dnar", "input dnar file (\"-\" for stdin)")

	nseCmd := flag.NewFlagSet("nix-store-export", flag.ExitOnError)
	nseCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: nix-store-export [options]\n\n")
		fmt.Fprintf(os.Stderr, "Convert a DNAR file into a nix-store --export compatible byte stream.\n")
		fmt.Fprintf(os.Stderr, "The output can be piped directly into 'nix-store --import'.\n\n")
		fmt.Fprintf(os.Stderr, "Example:\n")
		fmt.Fprintf(os.Stderr, "  dnar-unpack nix-store-export -input delta.dnar | nix-store --import\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		nseCmd.PrintDefaults()
	}

	var nseInput string
	nseCmd.StringVar(&nseInput, "input", "delta.dnar", "input dnar file (\"-\" for stdin)")

	var nseOutput string
	nseCmd.StringVar(&nseOutput, "output", "-", "output file (\"-\" for stdout)")

	flag.Parse()

	if len(os.Args) < 2 {
		flag.Usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "binary-cache":
		bcCmd.Parse(os.Args[2:])
		binaryCacheMain(bcInput, bcDir)

	case "nix-store-export":
		nseCmd.Parse(os.Args[2:])
		nixStoreExportMain(nseInput, nseOutput)

	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", os.Args[1])
		flag.Usage()
		os.Exit(1)
	}
}
