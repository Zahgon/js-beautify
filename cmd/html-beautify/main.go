// Command html-beautify is the Go port of js-beautify's html-beautify command.
package main

import (
	"os"

	beautify "github.com/beautifier/js-beautify-go"
)

func main() {
	os.Exit(beautify.CLI("html-beautify", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
