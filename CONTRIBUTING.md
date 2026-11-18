# Contributing


## Report Issues and Request Changes
If you find a bug, please report it, including environment and examples of current behavior and what you believe to be the correct behavior.  The clearer your description and information, the more likely it is someone will be able to make progress on it.  The default issue template will help guide you through this.

## How to Make Changes (Implement Fixes and New Features)
Fixes and enhancements are totally welcome.  We prefer you to file an issue before filing a PR, as this gives us chance to discuss design details, but feel free to dive right in.

### 0. Prerequisites for development

* Go 1.27 or later

If you encounter issues and cannot build, come chat on gitter.im.  We're happy to help.

### 1. Build and Test Locally
This repository holds the Go implementation of the beautifiers: the module `github.com/beautifier/js-beautify-go`
and the `js-beautify`, `css-beautify` and `html-beautify` commands.

* Familiarize yourself with the folder structure and code style before you dive in.
* Run `go build ./...` and `go test ./...` to build and run the tests.
* Run `gofmt -l .` and `go vet ./...` to run style checks.
* Add tests next to the code you change. Expected results come from the JavaScript release of js-beautify 2.0.3;
  each package's `testdata/` holds the recorder script that produced its fixtures.

### 2. Update Documentation and Tools
Update documentation as needed.  This includes files such as the README.md, internal command-line help, and file comments.

### 3. Submit a Pull Request

* Run `go vet ./...` and `go test ./...` locally after commit but before creation of Pull Request.  You may start a Pull Request even if this reports errors, but the PR will not be merged until all errors are fixed.
* Include description of changes. Include examples of input and expected output if possible.
* Pull requests must pass the `Go` workflow before being merged.


# Folders

## Root
The public package `beautify` (`beautify.go`): `JS`, `CSS`, `HTML` and `CLI`.

## `cmd`
The three commands.

## `internal`
The beautifiers (`javascript`, `css`, `html`, built on `core` and `jsrt`) and the command line (`cli` with
`nopt`, `configchain`, `editorconfig`, `minimatch`, `glob` and the Node.js behaviour it reproduces). See
`MIGRATION.md` for the full map.


# Branches and Tags

## Main
We use the `main` branch as the primary development branch.

## Release
We use the `release` branch to hold releases.

# Tags
Each release is has a tag created for it in the `release` branch when it is published.  The latest release is linked from the `README.md`.

## Attic
This project has been around for a while.  While some parts have improved significantly over time, others fell
into disrepair and were mothballed. All branches named `attic-*` are significantly out of date and kept for reference purposes only.

### PHP
`attic-php` contains a PHP implementation of the beautifier.
If you're interested in using it feel free.
If you plan to enhance it, please consider joining this project, and updating this version to match current functionality.

### Other Languages
Versions of the beautifier adapted to other languages are available on branch `attic-other`.
Take a look and feel free to resurrect them, but know it's pretty dusty back there.

### Generic Eval Unpacker
The `attic-genericeval` branch includes an unpacker that calls `eval` on whatever source is passed to it.
This file may be useful when working with source that unpacks itself when `eval` is called on it, but is also very unsafe.
We have isolated it on this separate branch to keep it from hurting the other children.

# How to publish a new version

* Close the Milestone for this release on github.
* Update CHANGELOG.md with the milestone description and a list of closed issues.
* Tag the release (`vX.Y.Z`) on `main`; the Go module proxy picks it up from the tag.

## Version Numbering Policy

This project follows semantic versioning for API compatibility, with additional guidance for formatting changes.

### What each version number means

* Major (`x.0.0`): API compatibility changes. We reserve major versions for intentional API-incompatible changes.
* Minor (`x.y.0`): Significant formatting fixes, behavior improvements, or new features/options that may affect output.
* Patch (`x.y.z`): Small, low-risk fixes that should not cause side effects.

### Output compatibility expectations

Formatting output for the same input may change in minor and patch releases.
For this project, output compatibility is not equivalent to API compatibility:

* API compatibility is represented by major versions.
* Formatting correctness and behavior improvements are delivered in minor/patch releases based on scope and risk.

### Rules for introducing formatting behavior changes

* Fixes for strictly incorrect formatting should be enabled by default.
* New formatting features should be introduced as options and must default to off, so valid input formatting does not change unexpectedly.
* We avoid unnecessary output churn and treat invalid input handling as best-effort.

### Guidance for consumers

If your workflows are sensitive to output differences, pin exact versions or use conservative ranges.
Using broad ranges (for example, `^x.y.z`) may pull in output-affecting updates by design.

