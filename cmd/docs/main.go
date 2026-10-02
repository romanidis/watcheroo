// Command docs writes wtr's command reference as markdown.
//
//	go run ./cmd/docs --dir docs
//
// The page is generated from the command's own help text, so it says what
// wtr --help says. It lives outside the wtr binary because wtr takes file
// names as arguments, and a docs subcommand would claim any file or directory
// called docs.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/romanidis/watcheroo/cmd"
	"github.com/spf13/cobra/doc"
)

func main() {
	dir := flag.String("dir", "docs", "where the pages go")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "making %s: %v\n", *dir, err)
		os.Exit(1)
	}
	root := cmd.NewRootCmd()
	// Without this every page ends in the date it was generated, and changes daily.
	root.DisableAutoGenTag = true
	if err := doc.GenMarkdownTree(root, *dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
