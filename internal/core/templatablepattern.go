package core

import "github.com/beautifier/js-beautify-go/internal/jsrt"

// TemplateLanguage names a templating language TemplatablePattern can step over.
type TemplateLanguage int

// The templating languages, in the order of the original's template_names.
const (
	Django TemplateLanguage = iota
	ERB
	Handlebars
	PHP
	Smarty
	Angular
	templateLanguageCount
)

var templateLanguageNames = [templateLanguageCount]jsrt.String{"django", "erb", "handlebars", "php", "smarty", "angular"}

// templateSet records a flag per templating language.
type templateSet [templateLanguageCount]bool

// templatePatterns are the readers of each kind of template tag.
type templatePatterns struct {
	handlebarsComment, handlebarsUnescaped, handlebars, php, erb             *Pattern
	django, djangoValue, djangoComment, smarty, smartyComment, smartyLiteral *Pattern
}

func newTemplatePatterns(input *InputScanner) *templatePatterns {
	pattern := NewPattern(input)
	re := func(s jsrt.String) *jsrt.Regexp { return compileCached(s, "") }
	return &templatePatterns{
		handlebarsComment:   pattern.StartingWith(re(`{{!--`)).UntilAfter(re(`--}}`)),
		handlebarsUnescaped: pattern.StartingWith(re(`{{{`)).UntilAfter(re(`}}}`)),
		handlebars:          pattern.StartingWith(re(`{{`)).UntilAfter(re(`}}`)),
		php:                 pattern.StartingWith(re(`<\?(?:[= ]|php)`)).UntilAfter(re(`\?>`)),
		erb:                 pattern.StartingWith(re(`<%[^%]`)).UntilAfter(re(`[^%]%>`)),
		// django conflicts with handlebars a bit.
		django:        pattern.StartingWith(re(`{%`)).UntilAfter(re(`%}`)),
		djangoValue:   pattern.StartingWith(re(`{{`)).UntilAfter(re(`}}`)),
		djangoComment: pattern.StartingWith(re(`{#`)).UntilAfter(re(`#}`)),
		smarty:        pattern.StartingWith(re(`{(?=[^}{\s\n])`)).UntilAfter(re(`[^\s\n]}`)),
		smartyComment: pattern.StartingWith(re(`{\*`)).UntilAfter(re(`\*}`)),
		smartyLiteral: pattern.StartingWith(re(`{literal}`)).UntilAfter(re(`{\/literal}`)),
	}
}

// TemplatablePattern is a Pattern that steps over template tags (<?php ?>, {{ }}, {% %}, ...)
// wherever it reads up to an until pattern, so that they never end a token early
// (js/src/core/templatablepattern.js). Languages can be disabled (not recognised at all) or
// excluded (recognised as boundaries but not read as templates).
type TemplatablePattern struct {
	Pattern
	templatePattern *jsrt.Regexp
	disabled        templateSet
	excluded        templateSet
	patterns        *templatePatterns
}

// NewTemplatablePattern returns a pattern reading from input with every language enabled.
func NewTemplatablePattern(input *InputScanner) *TemplatablePattern {
	return &TemplatablePattern{Pattern: Pattern{input: input}, patterns: newTemplatePatterns(input)}
}

// clone is _create: a copy sharing the template readers.
func (p *TemplatablePattern) clone() *TemplatablePattern {
	r := *p
	return &r
}

// UntilAfter is Pattern.UntilAfter for a TemplatablePattern.
func (p *TemplatablePattern) UntilAfter(pattern *jsrt.Regexp) *TemplatablePattern {
	r := p.clone()
	r.setUntil(pattern, true)
	r.update()
	return r
}

// Until is Pattern.Until for a TemplatablePattern.
func (p *TemplatablePattern) Until(pattern *jsrt.Regexp) *TemplatablePattern {
	r := p.clone()
	r.setUntil(pattern, false)
	r.update()
	return r
}

// StartingWith is Pattern.StartingWith for a TemplatablePattern.
func (p *TemplatablePattern) StartingWith(pattern *jsrt.Regexp) *TemplatablePattern {
	r := p.clone()
	r.setStartingWith(pattern)
	r.update()
	return r
}

// Matching is Pattern.Matching for a TemplatablePattern.
func (p *TemplatablePattern) Matching(pattern *jsrt.Regexp) *TemplatablePattern {
	r := p.clone()
	r.setMatching(pattern)
	r.update()
	return r
}

// Disable returns a copy that does not recognise language.
func (p *TemplatablePattern) Disable(language TemplateLanguage) *TemplatablePattern {
	r := p.clone()
	r.disabled[language] = true
	r.update()
	return r
}

// ReadOptions returns a copy that recognises exactly the languages listed in
// options.Templating.
func (p *TemplatablePattern) ReadOptions(options *Options) *TemplatablePattern {
	r := p.clone()
	for language, name := range templateLanguageNames {
		r.disabled[language] = !containsString(options.Templating, name)
	}
	r.update()
	return r
}

// Exclude returns a copy that does not read language's tags as templates.
func (p *TemplatablePattern) Exclude(language TemplateLanguage) *TemplatablePattern {
	r := p.clone()
	r.excluded[language] = true
	r.update()
	return r
}

// Read reads the pattern, stepping over every template tag on the way.
func (p *TemplatablePattern) Read() jsrt.String {
	var result jsrt.String
	if p.matchPattern != nil {
		result = p.input.Read(p.startingPattern, nil, false)
	} else {
		result = p.input.Read(p.startingPattern, p.templatePattern, false)
	}
	next := p.readTemplate()
	for next != "" {
		if p.matchPattern != nil {
			next += p.input.Read(p.matchPattern, nil, false)
		} else {
			next += p.input.ReadUntil(p.templatePattern, false)
		}
		result += next
		next = p.readTemplate()
	}

	if p.untilAfter {
		result += p.input.ReadUntilAfter(p.untilPattern)
	}
	return result
}

// update is _update: it rebuilds the pattern that finds the next template tag or the until
// pattern.
func (p *TemplatablePattern) update() {
	var items []jsrt.String

	if !p.disabled[PHP] {
		items = append(items, p.patterns.php.startingPattern.Source())
	}
	if !p.disabled[Handlebars] {
		items = append(items, p.patterns.handlebars.startingPattern.Source())
	}
	if !p.disabled[Angular] {
		// Handlebars ('{{' and '}}') are also special tokens in Angular.
		items = append(items, p.patterns.handlebars.startingPattern.Source())
	}
	if !p.disabled[ERB] {
		items = append(items, p.patterns.erb.startingPattern.Source())
	}
	if !p.disabled[Django] {
		items = append(items, p.patterns.django.startingPattern.Source())
		// django has separate patterns for values, comments and other sections.
		items = append(items, p.patterns.djangoValue.startingPattern.Source())
		items = append(items, p.patterns.djangoComment.startingPattern.Source())
	}
	if !p.disabled[Smarty] {
		items = append(items, p.patterns.smarty.startingPattern.Source())
	}

	if p.untilPattern != nil {
		items = append(items, p.untilPattern.Source())
	}
	p.templatePattern = regexpWithFlags("(?:"+jsrt.Join(items, "|")+")", false)
}

// readTemplate reads the template tag at the current position, or returns "".
func (p *TemplatablePattern) readTemplate() jsrt.String {
	var resultingString jsrt.String
	c := p.input.Peek(0)
	readFirst := func(patterns ...*Pattern) {
		for _, pat := range patterns {
			if resultingString != "" {
				return
			}
			resultingString = pat.Read()
		}
	}
	if c == "<" {
		peek1 := p.input.Peek(1)
		// Template tags are read as literals up to their closing delimiter.
		if !p.disabled[PHP] && !p.excluded[PHP] && peek1 == "?" {
			readFirst(p.patterns.php)
		}
		if !p.disabled[ERB] && !p.excluded[ERB] && peek1 == "%" {
			readFirst(p.patterns.erb)
		}
	} else if c == "{" {
		if !p.disabled[Handlebars] && !p.excluded[Handlebars] {
			readFirst(p.patterns.handlebarsComment, p.patterns.handlebarsUnescaped, p.patterns.handlebars)
		}
		if !p.disabled[Django] {
			// django conflicts with handlebars a bit.
			if !p.excluded[Django] && !p.excluded[Handlebars] {
				readFirst(p.patterns.djangoValue)
			}
			if !p.excluded[Django] {
				readFirst(p.patterns.djangoComment, p.patterns.django)
			}
		}
		if !p.disabled[Smarty] {
			// smarty cannot be enabled with django or handlebars enabled.
			if p.disabled[Django] && p.disabled[Handlebars] {
				readFirst(p.patterns.smartyComment, p.patterns.smartyLiteral, p.patterns.smarty)
			}
		}
	}
	return resultingString
}
