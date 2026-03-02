package app

import (
	"fmt"
	"os"
)

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: go run main.go <command> [options] [config.yml]")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  auth         Sign in once to save your shopping session.")
	fmt.Fprintln(os.Stderr, "  search       Find products and return structured product details.")
	fmt.Fprintln(os.Stderr, "  basket       View current basket, add items, or remove items.")
	fmt.Fprintln(os.Stderr, "  checkout     Attempt order placement and report checkout error details if shown.")
	fmt.Fprintln(os.Stderr, "\nGlobal Options:")
	fmt.Fprintln(os.Stderr, "  --help, -h   Show this help message and exit.")
	fmt.Fprintln(os.Stderr, "\nOptions for 'auth' command:")
	fmt.Fprintln(os.Stderr, "  --erase-data Force deletion of existing session data before authenticating.")
	fmt.Fprintln(os.Stderr, "\nArguments:")
	fmt.Fprintln(os.Stderr, "  [config.yml] (Optional) Path to the config file. Defaults to 'config.yml'.")
	fmt.Fprintln(os.Stderr, "  basket Returns current basket JSON.")
	fmt.Fprintln(os.Stderr, "  basket add <venue_slug> <item_id> Increases item quantity and prints updated basket JSON.")
	fmt.Fprintln(os.Stderr, "  basket remove <venue_slug> <item_id> Removes the item from basket and prints updated basket JSON.")
	fmt.Fprintln(os.Stderr, "  checkout <venue_slug> Attempts order placement and reports checkout errors when present.")
}
