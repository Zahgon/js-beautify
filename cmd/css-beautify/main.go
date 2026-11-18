// Command css-beautify is the Go port of js-beautify's css-beautify command.
package main

import (
	"os"

	beautify "github.com/beautifier/js-beautify-go"
)

func main() {
	os.Exit(beautify.CLI("css-beautify", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
