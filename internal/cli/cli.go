// Package cli ports js/src/cli.js, the js-beautify, css-beautify and html-beautify commands:
// option parsing (nopt), the configuration chain (command line, jsbeautify_* environment
// variables, --config, .jsbeautifyrc files, built-in defaults), EditorConfig support, file and
// glob selection, and the output, messages and exit statuses of the original running on Node.
package cli

import (
	"bytes"
	"io"
	"os"
	"os/user"
	"regexp"
	"strings"
	"syscall"

	"github.com/beautifier/js-beautify-go/internal/configchain"
	"github.com/beautifier/js-beautify-go/internal/editorconfig"
	"github.com/beautifier/js-beautify-go/internal/glob"
	"github.com/beautifier/js-beautify-go/internal/jsrt"
	"github.com/beautifier/js-beautify-go/internal/jsvalue"
	"github.com/beautifier/js-beautify-go/internal/nodefmt"
	"github.com/beautifier/js-beautify-go/internal/nodepath"
	"github.com/beautifier/js-beautify-go/internal/nopt"
)

// Version is the js-beautify release the commands report.
const Version = "2.0.3"

// Beautifiers are the three beautifiers the commands dispatch to.
type Beautifiers struct {
	JS, CSS, HTML func(source string, options map[string]any) (string, error)
}

// defaultsPath is where js-beautify reads its built-in defaults from; the chain report names it.
const defaultsPath = "js/lib/../config/defaults.json"

// defaultsJSON is the content of js-beautify's config/defaults.json.
const defaultsJSON = `{
    "indent_size": 4,
    "indent_char": " ",
    "indent_level": 0,
    "indent_with_tabs": false,
    "preserve_newlines": true,
    "max_preserve_newlines": 10,
    "jslint_happy": false,
    "space_after_named_function": false,
    "space_after_anon_function": false,
    "brace_style": "collapse",
    "keep_array_indentation": false,
    "keep_function_indentation": false,
    "space_before_conditional": true,
    "break_chained_methods": false,
    "eval_code": false,
    "unescape_strings": false,
    "wrap_line_length": 0,
    "indent_empty_lines": false,
    "templating": ["auto"]
}`

// exit ends the command with a status, as process.exit does.
type exit struct{ code int }

func (e exit) Error() string { return "exit" }

// thrown is a value the original throws that is not an Error (js-beautify throws strings).
type thrown struct{ msg string }

func (t thrown) Error() string { return t.msg }

// stdinJob is a '-' input: js-beautify registers its handlers while walking the files and
// beautifies standard input once the synchronous work is over.
type stdinJob struct {
	fileType any
	config   *jsvalue.Object
	outfile  any
}

type runner struct {
	name           string
	stdin          io.Reader
	stdout, stderr io.Writer
	b              Beautifiers
	cwd, home      string
	stdinJobs      []stdinJob
}

// Run runs the command called name with args, reading stdin and writing stdout and stderr,
// and returns its exit status.
func Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer, b Beautifiers) int {
	cwd, err := syscall.Getwd()
	if err != nil {
		cwd = "/"
	}
	r := &runner{name: name, stdin: stdin, stdout: stdout, stderr: stderr, b: b, cwd: cwd, home: osHomedir()}
	err = r.interpret(args)
	if e, ok := err.(exit); ok {
		return e.code
	}
	return 0
}

// osHomedir returns os.homedir(): $HOME when it is set, else the user's home directory.
func osHomedir() string {
	if home, ok := os.LookupEnv("HOME"); ok {
		return home
	}
	if u, err := user.Current(); err == nil {
		return u.HomeDir
	}
	return ""
}

func (r *runner) log(s string) {
	io.WriteString(r.stdout, s+"\n")
}

func (r *runner) logError(s string) {
	io.WriteString(r.stderr, s+"\n")
}

// consoleError prints an exception as console.error does: an Error is inspected, a thrown string
// is printed as it is.
func (r *runner) consoleError(err error) {
	switch e := err.(type) {
	case *nodefmt.Error:
		r.logError(e.Inspect())
	case *jsvalue.SyntaxError:
		r.logError((&nodefmt.Error{Name: "SyntaxError", Message: e.Message, Frame: "JSON.parse (<anonymous>)"}).Inspect())
	case thrown:
		r.logError(e.msg)
	default:
		r.logError(beautifierError(err).Inspect())
	}
}

// beautifierError turns an error returned by a beautifier into the exception the original
// throws: its name comes from a Name method when the error has one, and is "Error" otherwise.
func beautifierError(err error) *nodefmt.Error {
	name := "Error"
	if named, ok := err.(interface{ Name() string }); ok {
		name = named.Name()
	}
	return &nodefmt.Error{Name: name, Message: err.Error(), Frame: "makePretty (js/lib/cli.js)"}
}

func (r *runner) scriptHint() string {
	return "Run `" + r.name + " -h` for help."
}

func (r *runner) usage(errText string) {
	text := usageText(r.name)
	if errText != "" {
		r.logError(text + "\n" + errText + "\n")
		return
	}
	r.log(text)
}

func (r *runner) parser() *nopt.Parser {
	return &nopt.Parser{Types: knownOpts, Shorthands: shortHands, TypeDefs: typeDefs(r.cwd, r.home)}
}

func (r *runner) interpret(args []string) error {
	p := r.parser()
	parsed, err := p.Parse(args)
	if err != nil {
		r.usage(errorString(err))
		return exit{1}
	}

	if v, _ := parsed.Get("version"); jsvalue.Truthy(v) {
		r.log(Version)
		return exit{0}
	} else if v, _ := parsed.Get("help"); jsvalue.Truthy(v) {
		r.usage("")
		return exit{0}
	}

	configRecursive := r.findRecursive(r.cwd, ".jsbeautifyrc")
	configHome := r.verifyExists(nodepath.Join(getUserHome(), ".jsbeautifyrc"))

	parsedConfig, _ := parsed.Get("config")
	cfg, err := r.loadConfiguration(p, parsed, parsedConfig, configRecursive, configHome)
	if err != nil {
		r.consoleError(err)
		r.logError("Error while loading beautifier configuration.")
		r.logError("Configuration file chain included:")
		if jsvalue.Truthy(parsedConfig) {
			r.logError(jsvalue.ToString(parsedConfig))
		}
		if configRecursive != "" {
			r.logError(configRecursive)
		}
		if configHome != "" {
			r.logError(configHome)
		}
		r.logError(defaultsPath)
		r.logError(r.scriptHint())
		return exit{1}
	}

	if err := r.run(cfg); err != nil {
		if e, ok := err.(exit); ok {
			return e
		}
		r.consoleError(err)
		r.logError(r.scriptHint())
		return exit{1}
	}
	return r.processStdin()
}

// loadConfiguration builds the configuration from, by precedence, the command line, the
// jsbeautify_* environment variables, the --config file, the .jsbeautifyrc found from the working
// directory up, the one in the home directory, and the built-in defaults.
func (r *runner) loadConfiguration(p *nopt.Parser, parsed *jsvalue.Object, config any, recursive, home string) (*jsvalue.Object, error) {
	env := configchain.Env("jsbeautify_", os.Environ())
	if err := p.Clean(env); err != nil {
		return nil, err
	}
	defaults, err := jsvalue.ParseJSON(defaultsJSON)
	if err != nil {
		return nil, err
	}
	chain, err := configchain.New(r.cwd, parsed, env, config, nullable(recursive), nullable(home), defaults)
	if err != nil {
		return nil, err
	}
	return chain.Snapshot(), nil
}

// errorString returns String(err), as usage() shows a parsing exception.
func errorString(err error) string {
	if e, ok := err.(*nodefmt.Error); ok {
		return e.String()
	}
	return err.Error()
}

// nullable turns a path that was not found ("") into a value the chain skips.
func nullable(path string) any {
	if path == "" {
		return nil
	}
	return path
}

func getUserHome() string {
	if v := os.Getenv("USERPROFILE"); v != "" {
		return v
	}
	return os.Getenv("HOME")
}

// verifyExists returns fullPath when fs.existsSync finds it, else "".
func (r *runner) verifyExists(fullPath string) string {
	if _, err := os.Stat(nodepath.Resolve(r.cwd, fullPath)); err == nil {
		return fullPath
	}
	return ""
}

// findRecursive looks for fileName in dir and its ancestors.
func (r *runner) findRecursive(dir, fileName string) string {
	fullPath := nodepath.Join(dir, fileName)
	nextDir := nodepath.Dirname(dir)
	result := r.verifyExists(fullPath)
	if result == "" && nextDir != dir {
		result = r.findRecursive(nextDir, fileName)
	}
	return result
}

// run checks the type and the files, then processes every file in order.
func (r *runner) run(cfg *jsvalue.Object) error {
	r.checkType(cfg)
	if err := r.checkFiles(cfg); err != nil {
		return err
	}
	files, _ := cfg.Lookup("files")
	for _, f := range files.([]any) {
		if err := r.processInputSync(f.(string), cfg); err != nil {
			return err
		}
	}
	return nil
}

func (r *runner) scriptType() any {
	t, _, _ := strings.Cut(r.name, "-")
	switch t {
	case "js", "css", "html":
		return t
	}
	return nil
}

func (r *runner) checkType(cfg *jsvalue.Object) {
	if t, _ := cfg.Lookup("type"); !jsvalue.Truthy(t) {
		cfg.Set("type", r.scriptType())
	}
}

// stdinIsTTY returns process.stdin.isTTY.
func (r *runner) stdinIsTTY() bool {
	f, ok := r.stdin.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// checkFiles turns the file arguments into the list of inputs: globs are expanded, other paths
// must exist, '-' (or a missing path whose last part is '-') means standard input.
func (r *runner) checkFiles(cfg *jsvalue.Object) error {
	argvValue, _ := cfg.Lookup("argv")
	argv := argvValue.(*jsvalue.Object)
	remain, _ := argv.Get("remain")
	isTTY := r.stdinIsTTY()

	fileParams, _ := cfg.Lookup("files")
	if !jsvalue.Truthy(fileParams) {
		fileParams = []any{}
	}
	var params []any
	switch fp := fileParams.(type) {
	case []any:
		params = append(append(params, fp...), remain.([]any)...)
	case string:
		return nodefmt.NewError("TypeError", "file_params.forEach is not a function")
	default:
		return nodefmt.NewError("TypeError", "file_params.concat is not a function")
	}

	var files []any
	contains := func(p string) bool {
		for _, f := range files {
			if f == p {
				return true
			}
		}
		return false
	}
	hadGlob := false
	cfg.Set("files", files)
	for _, f := range params {
		if f == "-" {
			continue
		}
		var found []string
		magic, err := glob.HasMagic(f)
		if err != nil {
			return err
		}
		if magic {
			hadGlob = true
			found, err = glob.Sync(f, glob.Options{Cwd: r.cwd, Absolute: true, Ignore: []string{"**/node_modules/**", "**/.git/**"}})
			if err != nil {
				return err
			}
		} else {
			name, isString := f.(string)
			if !isString || !r.pathExists(name) {
				if !isString {
					return nodefmt.InvalidArgType("path", f, "Object.parse (node:path)")
				}
				if nodepath.Parse(name).Base != "-" {
					return thrown{`Unable to open path "` + name + `"`}
				}
				name = "-"
			}
			found = []string{name}
		}
		for _, file := range found {
			if file == "-" {
				files = append(files, file)
			} else if filePath := nodepath.Resolve(r.cwd, file); !contains(filePath) {
				files = append(files, filePath)
			}
		}
	}

	replace := func() { cfg.Set("replace", true) }
	if out, _ := cfg.Lookup("outfile"); isTTY && len(files) == 0 {
		if s, ok := out.(string); ok {
			if !r.pathExists(s) {
				return thrown{`Unable to open path "` + s + `"`}
			}
			files = append(files, s)
			replace()
		}
	}
	if hadGlob || len(files) > 1 {
		replace()
	}
	if len(files) == 0 && !hadGlob {
		files = append(files, "-")
	}
	cfg.Set("files", files)
	if isTTY && !hadGlob && contains("-") {
		return thrown{"Must pipe input or define at least one file."}
	}
	return nil
}

// pathExists is the fs.statSync check of testFilePath.
func (r *runner) pathExists(p string) bool {
	if p == "-" {
		return true
	}
	_, err := os.Stat(nodepath.Resolve(r.cwd, p))
	return err == nil
}

var beautifiableExt = regexp.MustCompile(`\.(js|css|html)$`)

// outputType is getOutputType: the extension of the output file, else of the input file, else
// the configured type.
func outputType(outfile any, filepath string, configType any) (any, error) {
	if jsvalue.Truthy(outfile) && beautifiableExt.MatchString(jsvalue.ToString(outfile)) {
		s, ok := outfile.(string)
		if !ok {
			return nil, nodefmt.NewError("TypeError", "outfile.split is not a function")
		}
		return s[strings.LastIndexByte(s, '.')+1:], nil
	} else if filepath != "-" && beautifiableExt.MatchString(filepath) {
		return filepath[strings.LastIndexByte(filepath, '.')+1:], nil
	} else if jsvalue.Truthy(configType) {
		return configType, nil
	}
	return nil, thrown{"Could not determine appropriate beautifier from file paths: " + filepath}
}

func (r *runner) processInputSync(filepath string, cfg *jsvalue.Object) error {
	config := cfg
	outfile, _ := config.Lookup("outfile")
	if replace, _ := config.Lookup("replace"); outfile == true || jsvalue.Truthy(replace) {
		outfile = filepath
	}
	configType, _ := config.Lookup("type")
	fileType, err := outputType(outfile, filepath, configType)
	if err != nil {
		return err
	}

	if ec, _ := config.Lookup("editorconfig"); jsvalue.Truthy(ec) {
		var ecPath any = filepath
		if filepath == "-" {
			if jsvalue.Truthy(outfile) {
				ecPath = outfile
			} else {
				ecPath = "stdin." + jsvalue.ToString(fileType)
			}
		}
		config = configchain.Copy(config)
		r.setEditorconfigOpts(ecPath, config)
	}

	if filepath == "-" {
		r.stdinJobs = append(r.stdinJobs, stdinJob{fileType, config, outfile})
		return nil
	}
	data, err := readFileUTF8(nodepath.Resolve(r.cwd, filepath))
	if err != nil {
		return err
	}
	return r.makePretty(fileType, data, config, outfile)
}

// readFileUTF8 is fs.readFileSync(path, 'utf8').
func readFileUTF8(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nodefmt.SysError(err, "open", path)
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		return "", nodefmt.SysError(err, "read", "")
	}
	return nodefmt.DecodeUTF8(buf.Bytes()), nil
}

// setEditorconfigOpts is set_file_editorconfig_opts: the EditorConfig properties for file
// override indentation, line length, final newline and line ending settings. A file that is
// not a string, or EditorConfig files that cannot be parsed, change nothing.
func (r *runner) setEditorconfigOpts(file any, config *jsvalue.Object) {
	name, ok := file.(string)
	if !ok {
		return
	}
	props, err := editorconfig.ParseSync(nodepath.Resolve(r.cwd, name))
	if err != nil {
		return
	}
	get := func(k string) any {
		v, ok := props.Get(k)
		if !ok {
			return jsvalue.Undefined
		}
		return v
	}
	switch get("indent_style") {
	case "tab":
		config.Set("indent_with_tabs", true)
	case "space":
		config.Set("indent_with_tabs", false)
	}
	if v := get("indent_size"); jsvalue.Truthy(v) {
		config.Set("indent_size", v)
	}
	if v := get("max_line_length"); jsvalue.Truthy(v) {
		if v == "off" {
			config.Set("wrap_line_length", 0.0)
		} else {
			config.Set("wrap_line_length", jsrt.ParseInt(jsrt.FromUTF8(jsvalue.ToString(v)), 10))
		}
	}
	switch get("insert_final_newline") {
	case true:
		config.Set("end_with_newline", true)
	case false:
		config.Set("end_with_newline", false)
	}
	switch get("end_of_line") {
	case "cr":
		config.Set("eol", "\r")
	case "lf":
		config.Set("eol", "\n")
	case "crlf":
		config.Set("eol", "\r\n")
	}
}

// beautifier returns beautify[fileType]: the beautifier the property names, or the TypeError of
// calling what is not a function.
func (r *runner) beautifier(fileType any) (func(string, map[string]any) (string, error), error) {
	switch jsvalue.ToString(fileType) {
	case "js", "js_beautify":
		return r.b.JS, nil
	case "css", "css_beautify":
		return r.b.CSS, nil
	case "html", "html_beautify":
		return r.b.HTML, nil
	case "caller", "arguments":
		return nil, nodefmt.NewError("TypeError", "'caller', 'callee', and 'arguments' properties may not be accessed on strict mode functions or the arguments objects for calls to them")
	}
	return nil, nodefmt.NewError("TypeError", "beautify[fileType] is not a function")
}

// makePretty beautifies code and writes the result; an exception from either step is reported
// and ends the command with status 1.
func (r *runner) makePretty(fileType any, code string, config *jsvalue.Object, outfile any) error {
	err := r.beautifyAndWrite(fileType, code, config, outfile)
	if err == nil {
		return nil
	}
	if e, ok := err.(exit); ok {
		return e
	}
	r.consoleError(err)
	return exit{1}
}

func (r *runner) beautifyAndWrite(fileType any, code string, config *jsvalue.Object, outfile any) error {
	beautify, err := r.beautifier(fileType)
	if err != nil {
		return err
	}
	pretty, err := beautify(code, beautifierOptions(config))
	if err != nil {
		return err
	}
	return r.writePretty(pretty, outfile, config)
}

// beautifierOptions converts the configuration into the options map the beautifiers take. The
// beautifiers normalize "-" in option names to "_", and where two spellings meet JavaScript keeps
// the one its for-in loop visits last; the configuration knows that order and a map does not,
// so the names are normalized here, in that order.
func beautifierOptions(config *jsvalue.Object) map[string]any {
	options := map[string]any{}
	for _, k := range config.ForInKeys() {
		v, _ := config.Lookup(k)
		name := strings.ReplaceAll(k, "-", "_")
		if jsvalue.IsUndefined(v) {
			delete(options, name)
			continue
		}
		options[name] = jsvalue.ToJSON(v)
	}
	return options
}

func (r *runner) writePretty(pretty string, outfile any, config *jsvalue.Object) error {
	if !jsvalue.Truthy(outfile) {
		io.WriteString(r.stdout, pretty)
		return nil
	}
	name, ok := outfile.(string)
	if !ok {
		return nodefmt.InvalidArgType("path", outfile, "Object.dirname (node:path)")
	}
	full := nodepath.Resolve(r.cwd, name)
	if err := mkdirp(nodepath.Dirname(name), nodepath.Dirname(full)); err != nil {
		return err
	}
	rel := nodepath.Relative(r.cwd, r.cwd, name)
	if !isFileDifferent(full, pretty) {
		r.logToStdout("beautified "+rel+" - unchanged", config)
		return nil
	}
	if err := os.WriteFile(full, []byte(pretty), 0o666); err != nil {
		return r.onOutputError(nodefmt.SysError(err, "open", name))
	}
	r.logToStdout("beautified "+rel, config)
	return nil
}

func isFileDifferent(path, expected string) bool {
	data, err := readFileUTF8(path)
	return err != nil || data != expected
}

// onOutputError skips an output file that is not writable and ends the command with status 0
// on any other write error.
func (r *runner) onOutputError(err *nodefmt.Error) error {
	if err.Prop("code") == "EACCES" {
		r.logError(jsvalue.ToString(err.Prop("path")) + " is not writable. Skipping!")
		return nil
	}
	r.consoleError(err)
	return exit{0}
}

func (r *runner) logToStdout(s string, config *jsvalue.Object) {
	if quiet, _ := config.Lookup("quiet"); !jsvalue.Truthy(quiet) {
		r.log(s)
	}
}

// processStdin runs the '-' inputs once standard input has been read.
func (r *runner) processStdin() error {
	if len(r.stdinJobs) == 0 {
		return nil
	}
	data, err := io.ReadAll(r.stdin)
	if err != nil || len(data) == 0 {
		// The original throws this string from its stdin 'end' handler, an uncaught exception.
		r.logError("Must pipe input or define at least one file.")
		return exit{1}
	}
	text := nodefmt.DecodeUTF8(data)
	for _, job := range r.stdinJobs {
		if err := r.makePretty(job.fileType, text, job.config, job.outfile); err != nil {
			return err
		}
	}
	return nil
}
