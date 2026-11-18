package core

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/beautifier/js-beautify-go/internal/jsrt"
)

// The scenarios in testdata/core.json.gz were run through js-beautify's own core classes by
// testdata/record.js; each test here replays them and compares every observable result.

// str is a JSON string decoded with lone surrogates preserved; a JSON null decodes as valid=false.
type str struct {
	s     jsrt.String
	valid bool
}

func (s *str) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = str{}
		return nil
	}
	v, err := jsrt.UnquoteJSON(b)
	*s = str{v, true}
	return err
}

type scenario struct {
	Kind       string
	Input      str
	Templating []string
	Until      str
	UntilAfter bool
	Starting   str
	Matching   str
	Exclude    []string
	Disable    []string
	Out        []json.RawMessage

	WS, NL str

	Text       str
	Directives map[string]str
	Ignored    str

	Options map[string]any
	Base    str
	Eol     str
	Ops     [][]json.RawMessage
	Code    str
	Err     string
}

func loadScenarios(t *testing.T, kind string) []scenario {
	t.Helper()
	f, err := os.Open("testdata/core.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var all []scenario
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatal(err)
	}
	var out []scenario
	for _, s := range all {
		if s.Kind == kind || (kind == "pattern" && s.Kind == "templatable") {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		t.Fatalf("no %s scenarios", kind)
	}
	return out
}

// drain reads with read until the input is exhausted; when read returns "" one code unit is
// consumed instead. It returns a JSON-like trace matching record.js.
func drain(input *InputScanner, read func() any) []any {
	var out []any
	for guard := 0; guard < 500; guard++ {
		r := read()
		if r == nil {
			next := input.Next()
			if next == "" {
				break
			}
			r = map[string]jsrt.String{"next": next}
		}
		out = append(out, r)
	}
	return out
}

// decodeTrace decodes a recorded trace into the shapes drain produces.
func decodeTrace(t *testing.T, raw []json.RawMessage, object func(map[string]json.RawMessage) any) []any {
	t.Helper()
	var out []any
	for _, m := range raw {
		if len(m) > 0 && m[0] == '"' {
			s, err := jsrt.UnquoteJSON(m)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(m, &fields); err != nil {
			t.Fatal(err)
		}
		if next, ok := fields["next"]; ok {
			s, err := jsrt.UnquoteJSON(next)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, map[string]jsrt.String{"next": s})
			continue
		}
		out = append(out, object(fields))
	}
	return out
}

var languageByName = map[string]TemplateLanguage{
	"django": Django, "erb": ERB, "handlebars": Handlebars, "php": PHP, "smarty": Smarty, "angular": Angular,
}

func optionalRegexp(s str) *jsrt.Regexp {
	if !s.valid {
		return nil
	}
	return jsrt.MustCompile(s.s, "")
}

func TestPatternScenarios(t *testing.T) {
	for i, sc := range loadScenarios(t, "pattern") {
		input := NewInputScanner(sc.Input.s)
		var read func() jsrt.String
		if sc.Kind == "pattern" {
			p := NewPattern(input)
			if sc.Starting.valid {
				p = p.StartingWith(optionalRegexp(sc.Starting))
			}
			if sc.Matching.valid {
				p = p.Matching(optionalRegexp(sc.Matching))
			}
			if sc.Until.valid {
				if sc.UntilAfter {
					p = p.UntilAfter(optionalRegexp(sc.Until))
				} else {
					p = p.Until(optionalRegexp(sc.Until))
				}
			}
			read = p.Read
		} else {
			raw := map[string]any{"templating": toAnySlice(sc.Templating)}
			opts, err := NewOptions(raw, "")
			if err != nil {
				t.Fatal(err)
			}
			p := NewTemplatablePattern(input).ReadOptions(opts)
			if sc.Starting.valid {
				p = p.StartingWith(optionalRegexp(sc.Starting))
			}
			if sc.Matching.valid {
				p = p.Matching(optionalRegexp(sc.Matching))
			}
			if sc.Until.valid {
				if sc.UntilAfter {
					p = p.UntilAfter(optionalRegexp(sc.Until))
				} else {
					p = p.Until(optionalRegexp(sc.Until))
				}
			}
			for _, l := range sc.Exclude {
				p = p.Exclude(languageByName[l])
			}
			for _, l := range sc.Disable {
				p = p.Disable(languageByName[l])
			}
			read = p.Read
		}
		got := drain(input, func() any {
			if s := read(); s != "" {
				return s
			}
			return nil
		})
		want := decodeTrace(t, sc.Out, func(map[string]json.RawMessage) any { return nil })
		if !reflect.DeepEqual(got, want) {
			t.Errorf("scenario %d (%s on %q): got %q, want %q", i, sc.Kind, sc.Input.s, got, want)
		}
	}
}

func toAnySlice(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}

type whitespaceRead struct {
	ws       jsrt.String
	newlines int
	before   jsrt.String
}

func TestWhitespaceScenarios(t *testing.T) {
	for i, sc := range loadScenarios(t, "whitespace") {
		input := NewInputScanner(sc.Input.s)
		ws := NewWhitespacePattern(input)
		if sc.WS.valid {
			ws = ws.Matching(sc.WS.s, sc.NL.s)
		}
		got := drain(input, func() any {
			r := ws.Read()
			if r == "" {
				return nil
			}
			return whitespaceRead{r, ws.NewlineCount, ws.WhitespaceBeforeToken}
		})
		want := decodeTrace(t, sc.Out, func(f map[string]json.RawMessage) any {
			var r struct {
				WS       str
				Newlines int
				Before   str
			}
			b, err := json.Marshal(f)
			if err == nil {
				err = json.Unmarshal(b, &r)
			}
			if err != nil {
				t.Fatal(err)
			}
			return whitespaceRead{r.WS.s, r.Newlines, r.Before.s}
		})
		if !reflect.DeepEqual(got, want) {
			t.Errorf("scenario %d on %q: got %v, want %v", i, sc.Input.s, got, want)
		}
	}
}

func TestTokenizerScenarios(t *testing.T) {
	for i, sc := range loadScenarios(t, "tokenizer") {
		tok := NewTokenizer(sc.Input.s)
		stream := tok.Tokenize(tok)
		var got []string
		for stream.HasNext() {
			tk := stream.Next()
			got = append(got, fmt.Sprintf("%s|%q|%d|%q", tk.Type, tk.Text, tk.Newlines, tk.WhitespaceBefore))
		}
		var want []string
		for _, raw := range sc.Out {
			var fields []json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			var typ string
			var text, before str
			var newlines int
			for _, e := range []error{json.Unmarshal(fields[0], &typ), text.UnmarshalJSON(fields[1]),
				json.Unmarshal(fields[2], &newlines), before.UnmarshalJSON(fields[3])} {
				if e != nil {
					t.Fatal(e)
				}
			}
			want = append(want, fmt.Sprintf("%s|%q|%d|%q", typ, text.s, newlines, before.s))
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("scenario %d on %q:\n got %v\nwant %v", i, sc.Input.s, got, want)
		}
	}
}

func TestDirectivesScenarios(t *testing.T) {
	d := NewDirectives(`\/\*`, `\*\/`)
	for i, sc := range loadScenarios(t, "directives") {
		got := d.GetDirectives(sc.Text.s)
		var want map[jsrt.String]jsrt.String
		if sc.Directives != nil {
			want = map[jsrt.String]jsrt.String{}
			for k, v := range sc.Directives {
				want[jsrt.FromUTF8(k)] = v.s
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("scenario %d: GetDirectives(%q) = %v, want %v", i, sc.Text.s, got, want)
		}
		if ignored := d.ReadIgnored(NewInputScanner(sc.Input.s)); ignored != sc.Ignored.s {
			t.Errorf("scenario %d: ReadIgnored(%q) = %q, want %q", i, sc.Input.s, ignored, sc.Ignored.s)
		}
	}
}

func TestOutputScenarios(t *testing.T) {
	for i, sc := range loadScenarios(t, "output") {
		opts, err := NewOptions(sc.Options, "")
		if err != nil {
			t.Fatal(err)
		}
		o, err := NewOutput(opts, sc.Base.s)
		if err != nil {
			if err.Error() != sc.Err {
				t.Errorf("scenario %d: NewOutput error %v, want %q", i, err, sc.Err)
			}
			continue
		}
		for j, op := range sc.Ops {
			var name string
			if err := json.Unmarshal(op[0], &name); err != nil {
				t.Fatal(err)
			}
			arg := func(k int, v any) {
				if err := json.Unmarshal(op[k], v); err != nil {
					t.Fatal(err)
				}
			}
			argStr := func(k int) jsrt.String {
				var s str
				if err := s.UnmarshalJSON(op[k]); err != nil {
					t.Fatal(err)
				}
				return s.s
			}
			var result any
			n := len(op)
			switch name {
			case "set_indent":
				var indent, align int
				arg(1, &indent)
				arg(2, &align)
				result = o.SetIndent(indent, align)
			case "newline":
				var force bool
				arg(1, &force)
				result = o.AddNewLine(force)
			case "space":
				o.SpaceBeforeToken = true
			case "nbsp":
				o.NonBreakingSpace = true
			case "trim":
				var eat bool
				arg(1, &eat)
				o.Trim(eat)
			case "raw":
				var newlines int
				arg(1, &newlines)
				o.AddRawToken(NewToken(TokenRaw, argStr(3), newlines, argStr(2)))
			case "remove_indent":
				var index int
				arg(1, &index)
				o.RemoveIndent(index)
			case "ensure_empty_line_above":
				o.EnsureEmptyLineAbove(argStr(1), argStr(2))
			case "wrap_point":
				o.SetWrapPoint()
			case "raw_mode":
				arg(1, &o.Raw)
			case "token":
				o.AddToken(argStr(1))
				result = o.PreviousTokenWrapped
			}
			var wantResult any
			var wantNewline, wantBlank bool
			var wantLines int
			arg(n-4, &wantResult)
			arg(n-3, &wantNewline)
			arg(n-2, &wantBlank)
			arg(n-1, &wantLines)
			if result != wantResult || o.JustAddedNewline() != wantNewline || o.JustAddedBlankline() != wantBlank ||
				o.GetLineNumber() != wantLines {
				t.Errorf("scenario %d op %d %s: got %v %v %v %d, want %v %v %v %d", i, j, name, result,
					o.JustAddedNewline(), o.JustAddedBlankline(), o.GetLineNumber(), wantResult, wantNewline, wantBlank, wantLines)
			}
		}
		code, err := o.GetCode(sc.Eol.s)
		switch {
		case sc.Err != "":
			if err == nil || err.Error() != sc.Err {
				t.Errorf("scenario %d: GetCode error %v, want %q", i, err, sc.Err)
			}
		case err != nil:
			t.Errorf("scenario %d: unexpected error %v", i, err)
		case code != sc.Code.s:
			t.Errorf("scenario %d (%v): GetCode = %q, want %q", i, sc.Options, code, sc.Code.s)
		}
	}
}
