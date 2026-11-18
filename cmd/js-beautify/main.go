// Command js-beautify is the Go port of js-beautify's js-beautify command.
package main

import (
	"os"

	beautify "github.com/beautifier/js-beautify-go"
)

func main() {
	os.Exit(beautify.CLI("js-beautify", os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
