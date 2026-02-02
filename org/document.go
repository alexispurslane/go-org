// Package org is an Org mode syntax processor.
//
// It parses plain text into an AST and can export it as HTML or pretty printed Org mode syntax.
// Further export formats can be defined using the Writer interface.
//
// You probably want to start with something like this:
//
//	input := strings.NewReader("Your Org mode input")
//	html, err := org.New().Parse(input, "./").Write(org.NewHTMLWriter())
//	if err != nil {
//	    log.Fatalf("Something went wrong: %s", err)
//	}
//	log.Print(html)
package org

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// Position represents the location of a node in the source text.
type Position struct {
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

type Configuration struct {
	MaxEmphasisNewLines int                                   // Maximum number of newlines inside an emphasis. See org-emphasis-regexp-components newline.
	AutoLink            bool                                  // Try to convert text passages that look like hyperlinks into hyperlinks.
	DefaultSettings     map[string]string                     // Default values for settings that are overriden by setting the same key in BufferSettings.
	Log                 *log.Logger                           // Log is used to print warnings during parsing.
	ReadFile            func(filename string) ([]byte, error) // ReadFile is used to read e.g. #+INCLUDE files.
	ResolveLink         func(protocol string, description []Node, link string) Node
}

// Document contains the parsing results and a pointer to the Configuration.
type Document struct {
	*Configuration
	Path           string // Path of the file containing the parse input - used to resolve relative paths during parsing (e.g. INCLUDE).
	tokens         []token
	baseLvl        int
	Macros         map[string]string
	Links          map[string]string
	Nodes          []Node
	NamedNodes     map[string]Node
	Outline        Outline           // Outline is a Table Of Contents for the document and contains all sections (headline + content).
	BufferSettings map[string]string // Settings contains all settings that were parsed from keywords.
	Errors         []*ParseError     // Structured parsing errors with position information
	Pos            Position          // Position tracks the location of this document in the source
}

// Node represents a parsed node of the document.
type Node interface {
	String() string        // String returns the pretty printed Org mode string for the node (see OrgWriter).
	Copy() Node            // Copy returns a deep copy of the node.
	Range(func(Node) bool) // Range iterates over all children of the node. Stops if the function returns false.
	Position() Position    // Position returns the position of the node in the source text.
}

// NOTE: the reason I decided to do a Range method instead of a Children getter
// is that a node may have different properties on it that function as a
// children property, so if we do a Children getter do we either have it *only
// return the literal Children property if present*, or do we have it
// *transparently append all the properties that have children*? At least with
// Range, the interface isn't lying, and it's clear that it might be going over
// multiple things, when you have to append all its results together to get a
// full children list. Idk.

type lexFn = func(line string) (t token, ok bool)
type parseFn = func(*Document, int, stopFn) (int, Node)
type stopFn = func(*Document, int) bool

type token struct {
	kind     string
	lvl      int
	content  string
	matches  []string
	line     int
	startCol int
	endCol   int
}

var lexFns = []lexFn{
	lexHeadline,
	lexDrawer,
	lexBlock,
	lexResult,
	lexList,
	lexTable,
	lexHorizontalRule,
	lexKeywordOrComment,
	lexFootnoteDefinition,
	lexExample,
	lexLatexBlock,
	lexText,
}

var nilToken = token{kind: "nil", lvl: -1, content: "", matches: nil}
var orgWriterMutex = sync.Mutex{}
var orgWriter = NewOrgWriter()

// New returns a new Configuration with (hopefully) sane defaults.
func New() *Configuration {
	return &Configuration{
		AutoLink:            true,
		MaxEmphasisNewLines: 1,
		DefaultSettings: map[string]string{
			"TODO":         "TODO | DONE",
			"EXCLUDE_TAGS": "noexport",
			"OPTIONS":      "toc:t <:t e:t f:t pri:t todo:t tags:t title:t ealb:nil",
		},
		Log:      log.New(os.Stderr, "go-org: ", 0),
		ReadFile: os.ReadFile,
		ResolveLink: func(protocol string, description []Node, link string) Node {
			return RegularLink{Protocol: protocol, Description: description, URL: link, AutoLink: false}
		},
	}
}

// String returns the pretty printed Org mode string for the given nodes (see OrgWriter).
func String(nodes ...Node) string {
	orgWriterMutex.Lock()
	defer orgWriterMutex.Unlock()
	return orgWriter.WriteNodesAsString(nodes...)
}

// CopyNodes returns a deep copy of a slice of nodes.
func CopyNodes(nodes []Node) []Node {
	if nodes == nil {
		return nil
	}
	copied := make([]Node, len(nodes))
	for i, n := range nodes {
		copied[i] = n.Copy()
	}
	return copied
}

// Write is called after with an instance of the Writer interface to export a parsed Document into another format.
func (d *Document) Write(w Writer) (out string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("could not write output: %s", recovered)
		}
	}()
	if d.HasErrors() {
		return "", d.Errors[0]
	} else if d.Nodes == nil {
		return "", fmt.Errorf("could not write output: parse was not called")
	}
	w.Before(d)
	WriteNodes(w, d.Nodes...)
	w.After(d)
	return w.String(), err
}

// Parse parses the input into an AST (and some other helpful fields like Outline).
// To allow method chaining, errors are stored in document.Error rather than being returned.
func (c *Configuration) Parse(input io.Reader, path string) (d *Document) {
	outlineSection := &Section{}
	d = &Document{
		Configuration:  c,
		Outline:        Outline{outlineSection, outlineSection, 0},
		BufferSettings: map[string]string{},
		NamedNodes:     map[string]Node{},
		Links:          map[string]string{},
		Macros:         map[string]string{},
		Path:           path,
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			d.AddError(ErrorTypeInvalidStructure, "parse panic", d.Pos, token{}, fmt.Errorf("recovered from panic: %v", recovered))
		}
	}()
	if d.tokens != nil {
		d.AddError(ErrorTypeValidation, "parse called multiple times", d.Pos, token{}, nil)
		return nil
	}
	d.tokenize(input)
	_, nodes := d.parseMany(0, func(d *Document, i int) bool { return i >= len(d.tokens) })
	d.Nodes = nodes
	return d
}

// Silent disables all logging of warnings during parsing.
func (c *Configuration) Silent() *Configuration {
	c.Log = log.New(io.Discard, "", 0)
	return c
}

func (d *Document) tokenize(input io.Reader) {
	d.tokens = []token{}
	scanner := bufio.NewScanner(input)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		tok, ok := tokenize(line)
		if !ok {
			pos := Position{StartLine: lineNum, StartColumn: 1, EndLine: lineNum, EndColumn: len(line) + 1}
			d.AddError(ErrorTypeTokenization, "could not lex line", pos, token{line: lineNum}, fmt.Errorf("no lexer matched: %q", line))
			lineNum++
			continue
		}
		tok.line = lineNum
		tok.startCol = 0
		tok.endCol = len(line)
		d.tokens = append(d.tokens, tok)
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		d.AddError(ErrorTypeIO, "tokenization failed", Position{StartLine: lineNum, StartColumn: 0, EndLine: lineNum, EndColumn: 0}, token{line: lineNum}, err)
	}
}

// Get returns the value for key in BufferSettings or DefaultSettings if key does not exist in the former
func (d *Document) Get(key string) string {
	if v, ok := d.BufferSettings[key]; ok {
		return v
	}
	if v, ok := d.DefaultSettings[key]; ok {
		return v
	}
	return ""
}

// GetOption returns the value associated to the export option key
// Currently supported options:
// - < (export timestamps)
// - e (export org entities)
// - f (export footnotes)
// - title (export title)
// - toc (export table of content. an int limits the included org headline lvl)
// - todo (export headline todo status)
// - pri (export headline priority)
// - tags (export headline tags)
// - ealb (non-standard) (export with east asian line breaks / ignore line breaks between multi-byte characters)
// see https://orgmode.org/manual/Export-Settings.html for more information
func (d *Document) GetOption(key string) string {
	get := func(settings map[string]string) string {
		for _, field := range strings.Fields(settings["OPTIONS"]) {
			if strings.HasPrefix(field, key+":") {
				return field[len(key)+1:]
			}
		}
		return ""
	}
	value := get(d.BufferSettings)
	if value == "" {
		value = get(d.DefaultSettings)
	}
	if value == "" {
		value = "nil"
		d.Log.Printf("Missing value for export option %s", key)
	}
	return value
}

func (d *Document) parseOne(i int, stop stopFn) (consumed int, node Node) {
	switch d.tokens[i].kind {
	case "unorderedList", "orderedList":
		consumed, node = d.parseList(i, stop)
	case "tableRow", "tableSeparator":
		consumed, node = d.parseTable(i, stop)
	case "beginBlock":
		consumed, node = d.parseBlock(i, stop)
	case "beginLatexBlock":
		consumed, node = d.parseLatexBlock(i, stop)
	case "result":
		consumed, node = d.parseResult(i, stop)
	case "beginDrawer":
		consumed, node = d.parseDrawer(i, stop)
	case "text":
		if d.tokens[i].content == "" {
			return 1, nil // Skip blank lines
		}
		consumed, node = d.parseParagraph(i, stop)
	case "example":
		consumed, node = d.parseExample(i, stop)
	case "horizontalRule":
		consumed, node = d.parseHorizontalRule(i, stop)
	case "comment":
		consumed, node = d.parseComment(i, stop)
	case "keyword":
		consumed, node = d.parseKeyword(i, stop)
	case "headline":
		consumed, node = d.parseHeadline(i, stop)
	case "footnoteDefinition":
		consumed, node = d.parseFootnoteDefinition(i, stop)
	}

	if consumed != 0 {
		return consumed, node
	}
	d.AddError(ErrorTypeUnexpectedToken, "could not parse token", getPositionFromToken(d.tokens[i]), d.tokens[i], fmt.Errorf("no parser matched token kind %q", d.tokens[i].kind))
	m := plainTextRegexp.FindStringSubmatch(d.tokens[i].matches[0])
	d.tokens[i] = token{kind: "text", lvl: len(m[1]), content: m[2], matches: m}
	return d.parseOne(i, stop)
}

func (d *Document) parseMany(i int, stop stopFn) (int, []Node) {
	start, nodes := i, []Node{}
	for i < len(d.tokens) && !stop(d, i) {
		consumed, node := d.parseOne(i, stop)
		i += consumed
		if node != nil {
			nodes = append(nodes, node)
		}
	}
	return i - start, nodes
}

func (d *Document) addHeadline(headline *Headline) int {
	current := &Section{Headline: headline}
	d.Outline.last.add(current)
	if !headline.IsExcluded(d) {
		d.Outline.count++
	}
	d.Outline.last = current
	return d.Outline.count
}

func tokenize(line string) (token, bool) {
	for _, lexFn := range lexFns {
		if token, ok := lexFn(line); ok {
			return token, true
		}
	}
	return nilToken, false
}
