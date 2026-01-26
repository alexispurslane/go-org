package org

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Text struct {
	Content string
	IsRaw   bool
	Pos     Position
}

type LineBreak struct {
	Count                      int
	BetweenMultibyteCharacters bool
	Pos                        Position
}
type ExplicitLineBreak struct {
	Pos Position
}

type StatisticToken struct {
	Content string
	Pos     Position
}

type Timestamp struct {
	Time     time.Time
	IsDate   bool
	Interval string
	Pos      Position
}

type Emphasis struct {
	Kind    string
	Content []Node
	Pos     Position
}

type InlineBlock struct {
	Name       string
	Parameters []string
	Children   []Node
	Pos        Position
}

type LatexFragment struct {
	OpeningPair string
	ClosingPair string
	Content     []Node
	Pos         Position
}

type FootnoteLink struct {
	Name       string
	Definition *FootnoteDefinition
	Pos        Position
}

type RegularLink struct {
	Protocol    string
	Description []Node
	URL         string
	AutoLink    bool
	Pos         Position
}

type Macro struct {
	Name       string
	Parameters []string
	Pos        Position
}

var validURLCharacters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~:/?#[]@!$&'()*+,;="
var autolinkProtocols = regexp.MustCompile(`^(https?|ftp|file)$`)
var imageExtensionRegexp = regexp.MustCompile(`(?i)^[.](png|gif|jpe?g|svg|tiff?|webp|x[bp]m|p[bgpn]m)$`)
var videoExtensionRegexp = regexp.MustCompile(`(?i)^[.](webm|mp4)$`)

var subScriptSuperScriptRegexp = regexp.MustCompile(`^([_^]){([^{}]+?)}`)
var timestampRegexp = regexp.MustCompile(`^<(\d{4}-\d{2}-\d{2})( [A-Za-z]+)?( \d{2}:\d{2})?( \+\d+[dwmy])?>`)
var footnoteRegexp = regexp.MustCompile(`^\[fn:([\w-]*?)(:(.*?))?\]`)
var statisticsTokenRegexp = regexp.MustCompile(`^\[(\d+/\d+|\d+%)\]`)
var latexFragmentRegexp = regexp.MustCompile(`(?s)^\\begin{(\w+)}(.*)\\end{(\w+)}`)
var inlineBlockRegexp = regexp.MustCompile(`src_(\w+)(\[([^\]]*)\])?{([^}]*)}`)
var inlineExportBlockRegexp = regexp.MustCompile(`@@(\w+):(.*?)@@`)
var macroRegexp = regexp.MustCompile(`{{{(.*)\((.*)\)}}}`)

var timestampFormat = "2006-01-02 Mon 15:04"
var datestampFormat = "2006-01-02 Mon"

// calculatePosition computes a Position from a base offset and character offset
func calculatePosition(input string, startLine, startColumn int, charOffset int) Position {
	line := startLine
	col := startColumn

	for i := 0; i < charOffset && i < len(input); i++ {
		if input[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}

	return Position{
		StartLine:   line,
		StartColumn: col,
		EndLine:     line,
		EndColumn:   col,
	}
}

// positionFromChars returns a Position spanning from startOffset to endOffset
func positionFromChars(input string, startLine, startColumn int, startOffset, endOffset int) Position {
	start := calculatePosition(input, startLine, startColumn, startOffset)
	end := calculatePosition(input, startLine, startColumn, endOffset)
	return Position{
		StartLine:   start.StartLine,
		StartColumn: start.StartColumn,
		EndLine:     end.StartLine,
		EndColumn:   end.StartColumn,
	}
}

var latexFragmentPairs = map[string]string{
	`\(`: `\)`,
	`\[`: `\]`,
	`$$`: `$$`,
	`$`:  `$`,
}

// parseInline parses inline content without position tracking (legacy)
func (d *Document) parseInline(input string) (nodes []Node) {
	return d.parseInlineWithPos(input, 0, 0)
}

// parseInlineWithPos parses inline content with position tracking
func (d *Document) parseInlineWithPos(input string, startLine, startColumn int) (nodes []Node) {
	previous, current := 0, 0
	for current < len(input) {
		rewind, consumed, node := 0, 0, (Node)(nil)
		switch input[current] {
		case '^':
			consumed, node = d.parseSubOrSuperScriptWithPos(input, current, startLine, startColumn)
		case '_':
			rewind, consumed, node = d.parseSubScriptOrEmphasisOrInlineBlockWithPos(input, current, startLine, startColumn)
		case '@':
			consumed, node = d.parseInlineExportBlockWithPos(input, current, startLine, startColumn)
		case '*', '/', '+':
			consumed, node = d.parseEmphasisWithPos(input, current, false, startLine, startColumn)
		case '=', '~':
			consumed, node = d.parseEmphasisWithPos(input, current, true, startLine, startColumn)
		case '[':
			consumed, node = d.parseOpeningBracketWithPos(input, current, startLine, startColumn)
		case '{':
			consumed, node = d.parseMacroWithPos(input, current, startLine, startColumn)
		case '<':
			consumed, node = d.parseTimestampWithPos(input, current, startLine, startColumn)
		case '\\':
			consumed, node = d.parseExplicitLineBreakOrLatexFragmentWithPos(input, current, startLine, startColumn)
		case '$':
			consumed, node = d.parseLatexFragmentWithPos(input, current, 1, startLine, startColumn)
		case '\n':
			consumed, node = d.parseLineBreakWithPos(input, current, startLine, startColumn)
		case ':':
			rewind, consumed, node = d.parseAutoLink(input, current)
		}
		current -= rewind
		if consumed != 0 {
			if current > previous {
				textPos := positionFromChars(input, startLine, startColumn, previous, current)
				nodes = append(nodes, Text{Content: input[previous:current], IsRaw: false, Pos: textPos})
			}
			if node != nil {
				nodes = append(nodes, node)
			}
			current += consumed
			previous = current
		} else {
			current++
		}
	}

	if previous < len(input) {
		textPos := positionFromChars(input, startLine, startColumn, previous, len(input))
		nodes = append(nodes, Text{Content: input[previous:], IsRaw: false, Pos: textPos})
	}
	return nodes
}

func (d *Document) parseRawInline(input string) (nodes []Node) {
	return d.parseRawInlineWithPos(input, 0, 0)
}

func (d *Document) parseRawInlineWithPos(input string, startLine, startColumn int) (nodes []Node) {
	previous, current := 0, 0
	for current < len(input) {
		if input[current] == '\n' {
			consumed, node := d.parseLineBreakWithPos(input, current, startLine, startColumn)
			if current > previous {
				textPos := positionFromChars(input, startLine, startColumn, previous, current)
				nodes = append(nodes, Text{Content: input[previous:current], IsRaw: true, Pos: textPos})
			}
			nodes = append(nodes, node)
			current += consumed
			previous = current
		} else {
			current++
		}
	}
	if previous < len(input) {
		textPos := positionFromChars(input, startLine, startColumn, previous, len(input))
		nodes = append(nodes, Text{Content: input[previous:], IsRaw: true, Pos: textPos})
	}
	return nodes
}

func (d *Document) parseLineBreak(input string, start int) (int, Node) {
	return d.parseLineBreakWithPos(input, start, 0, 0)
}

func (d *Document) parseLineBreakWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	i := start
	for ; i < len(input) && input[i] == '\n'; i++ {
	}
	_, beforeLen := utf8.DecodeLastRuneInString(input[:start])
	_, afterLen := utf8.DecodeRuneInString(input[i:])
	consumed := i - start
	pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
	return consumed, LineBreak{Count: consumed, BetweenMultibyteCharacters: beforeLen > 1 && afterLen > 1, Pos: pos}
}

func (d *Document) parseInlineBlock(input string, start int) (int, int, Node) {
	return d.parseInlineBlockWithPos(input, start, 0, 0)
}

func (d *Document) parseInlineBlockWithPos(input string, start int, startLine, startColumn int) (int, int, Node) {
	if !(strings.HasSuffix(input[:start], "src") && (start-4 < 0 || unicode.IsSpace(rune(input[start-4])))) {
		return 0, 0, nil
	}
	if m := inlineBlockRegexp.FindStringSubmatch(input[start-3:]); m != nil {
		consumed := len(m[0])
		pos := positionFromChars(input, startLine, startColumn, start-3, start+consumed)

		return 3, consumed, InlineBlock{Name: "src", Parameters: strings.Fields(m[1] + " " + m[3]), Children: d.parseRawInline(m[4]), Pos: pos}
	}
	return 0, 0, nil
}

func (d *Document) parseInlineExportBlock(input string, start int) (int, Node) {
	return d.parseInlineExportBlockWithPos(input, start, 0, 0)
}

func (d *Document) parseInlineExportBlockWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := inlineExportBlockRegexp.FindStringSubmatch(input[start:]); m != nil {
		consumed := len(m[0])
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		return consumed, InlineBlock{Name: "export", Parameters: m[1:2], Children: d.parseRawInline(m[2]), Pos: pos}
	}
	return 0, nil
}

func (d *Document) parseExplicitLineBreakOrLatexFragment(input string, start int) (int, Node) {
	return d.parseExplicitLineBreakOrLatexFragmentWithPos(input, start, 0, 0)
}

func (d *Document) parseExplicitLineBreakOrLatexFragmentWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	switch {
	case start+2 >= len(input):
	case input[start+1] == '\\' && start != 0 && input[start-1] != '\n':
		for i := start + 2; i <= len(input)-1 && unicode.IsSpace(rune(input[i])); i++ {
			if input[i] == '\n' {
				consumed := i + 1 - start
				pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
				return consumed, ExplicitLineBreak{Pos: pos}
			}
		}
	case input[start+1] == '(' || input[start+1] == '[':
		return d.parseLatexFragmentWithPos(input, start, 2, startLine, startColumn)
	case strings.Index(input[start:], `\begin{`) == 0:
		if m := latexFragmentRegexp.FindStringSubmatch(input[start:]); m != nil {
			if open, content, close := m[1], m[2], m[3]; open == close {
				openingPair, closingPair := `\begin{`+open+`}`, `\end{`+close+`}`
				i := strings.Index(input[start:], closingPair)
				consumed := i + len(closingPair)
				pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
				return consumed, LatexFragment{OpeningPair: openingPair, ClosingPair: closingPair, Content: d.parseRawInline(content), Pos: pos}
			}
		}
	}
	return 0, nil
}

func (d *Document) parseLatexFragment(input string, start int, pairLength int) (int, Node) {
	return d.parseLatexFragmentWithPos(input, start, pairLength, 0, 0)
}

func (d *Document) parseLatexFragmentWithPos(input string, start int, pairLength int, startLine, startColumn int) (int, Node) {
	if start+2 >= len(input) {
		return 0, nil
	}
	if pairLength == 1 && input[start:start+2] == "$$" {
		pairLength = 2
	}
	openingPair := input[start : start+pairLength]
	closingPair := latexFragmentPairs[openingPair]
	if i := strings.Index(input[start+pairLength:], closingPair); i != -1 {
		content := d.parseRawInline(input[start+pairLength : start+pairLength+i])
		consumed := i + pairLength + pairLength
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		return consumed, LatexFragment{OpeningPair: openingPair, ClosingPair: closingPair, Content: content, Pos: pos}
	}
	return 0, nil
}

func (d *Document) parseSubOrSuperScript(input string, start int) (int, Node) {
	return d.parseSubOrSuperScriptWithPos(input, start, 0, 0)
}

func (d *Document) parseSubOrSuperScriptWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := subScriptSuperScriptRegexp.FindStringSubmatch(input[start:]); m != nil {
		consumed := len(m[2]) + 3
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		contentPos := positionFromChars(input, startLine, startColumn, start+2, start+2+len(m[2]))
		content := []Node{Text{Content: m[2], IsRaw: false, Pos: contentPos}}
		return consumed, Emphasis{Kind: m[1] + "{}", Content: content, Pos: pos}
	}
	return 0, nil
}

func (d *Document) parseSubScriptOrEmphasisOrInlineBlock(input string, start int) (int, int, Node) {
	return d.parseSubScriptOrEmphasisOrInlineBlockWithPos(input, start, 0, 0)
}

func (d *Document) parseSubScriptOrEmphasisOrInlineBlockWithPos(input string, start int, startLine, startColumn int) (int, int, Node) {
	if rewind, consumed, node := d.parseInlineBlockWithPos(input, start, startLine, startColumn); consumed != 0 {
		return rewind, consumed, node
	} else if consumed, node := d.parseSubOrSuperScriptWithPos(input, start, startLine, startColumn); consumed != 0 {
		return 0, consumed, node
	}
	consumed, node := d.parseEmphasisWithPos(input, start, false, startLine, startColumn)
	return 0, consumed, node
}

func (d *Document) parseOpeningBracket(input string, start int) (int, Node) {
	return d.parseOpeningBracketWithPos(input, start, 0, 0)
}

func (d *Document) parseOpeningBracketWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if len(input[start:]) >= 2 && input[start] == '[' && input[start+1] == '[' {
		return d.parseRegularLinkWithPos(input, start, startLine, startColumn)
	} else if footnoteRegexp.MatchString(input[start:]) {
		return d.parseFootnoteReferenceWithPos(input, start, startLine, startColumn)
	} else if statisticsTokenRegexp.MatchString(input[start:]) {
		return d.parseStatisticTokenWithPos(input, start, startLine, startColumn)
	}
	return 0, nil
}

func (d *Document) parseMacro(input string, start int) (int, Node) {
	return d.parseMacroWithPos(input, start, 0, 0)
}

func (d *Document) parseMacroWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := macroRegexp.FindStringSubmatch(input[start:]); m != nil {
		consumed := len(m[0])
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		return consumed, Macro{Name: m[1], Parameters: strings.Split(m[2], ","), Pos: pos}
	}
	return 0, nil
}

func (d *Document) parseFootnoteReference(input string, start int) (int, Node) {
	return d.parseFootnoteReferenceWithPos(input, start, 0, 0)
}

func (d *Document) parseFootnoteReferenceWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := footnoteRegexp.FindStringSubmatch(input[start:]); m != nil {
		name, definition := m[1], m[3]
		if name == "" && definition == "" {
			return 0, nil
		}
		link := FootnoteLink{Name: name, Definition: nil}
		if definition != "" {
			link.Definition = &FootnoteDefinition{Name: name, Children: []Node{Paragraph{Children: d.parseInlineWithPos(definition, startLine, startColumn+start+len(name)+5), Pos: Position{}}}, Inline: true}
		}
		consumed := len(m[0])
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		link.Pos = pos
		return consumed, link
	}
	return 0, nil
}

func (d *Document) parseStatisticToken(input string, start int) (int, Node) {
	return d.parseStatisticTokenWithPos(input, start, 0, 0)
}

func (d *Document) parseStatisticTokenWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := statisticsTokenRegexp.FindStringSubmatch(input[start:]); m != nil {
		consumed := len(m[1]) + 2
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		return consumed, StatisticToken{Content: m[1], Pos: pos}
	}
	return 0, nil
}

func (d *Document) parseAutoLink(input string, start int) (int, int, Node) {
	return d.parseAutoLinkWithPos(input, start, 0, 0)
}

func (d *Document) parseAutoLinkWithPos(input string, start int, startLine, startColumn int) (int, int, Node) {
	if !d.AutoLink || start == 0 || len(input[start:]) < 3 || input[start:start+3] != "://" {
		return 0, 0, nil
	}
	protocolStart, protocol := start-1, ""
	for ; protocolStart > 0; protocolStart-- {
		if !unicode.IsLetter(rune(input[protocolStart])) {
			protocolStart++
			break
		}
	}
	if m := autolinkProtocols.FindStringSubmatch(input[protocolStart:start]); m != nil {
		protocol = m[1]
	} else {
		return 0, 0, nil
	}
	end := start
	for ; end < len(input) && strings.ContainsRune(validURLCharacters, rune(input[end])); end++ {
	}
	path := input[start:end]
	if path == "://" {
		return 0, 0, nil
	}
	pos := positionFromChars(input, startLine, startColumn, start-len(protocol), start+len(path))
	// pos for autolink covers the entire URL including protocol
	rl := RegularLink{Protocol: protocol, Description: nil, URL: protocol + path, AutoLink: true, Pos: pos}
	return len(protocol), len(path + protocol), rl
}

func (d *Document) parseRegularLink(input string, start int) (int, Node) {
	return d.parseRegularLinkWithPos(input, start, 0, 0)
}

func (d *Document) parseRegularLinkWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	input = input[start:]
	if len(input) < 3 || input[:2] != "[[" || input[2] == '[' {
		return 0, nil
	}
	end := strings.Index(input, "]]")
	if end == -1 {
		return 0, nil
	}
	rawLinkParts := strings.Split(input[2:end], "][")
	description, link := ([]Node)(nil), rawLinkParts[0]
	if len(rawLinkParts) == 2 {
		link, description = rawLinkParts[0], d.parseInlineWithPos(rawLinkParts[1], startLine, startColumn+start+2)
	}
	if strings.ContainsRune(link, '\n') {
		return 0, nil
	}
	consumed := end + 2
	protocol, linkParts := "", strings.SplitN(link, ":", 2)
	if len(linkParts) == 2 {
		protocol = linkParts[0]
	}
	pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
	linkNode := d.ResolveLink(protocol, description, link)
	if rl, ok := linkNode.(RegularLink); ok {
		rl.Pos = pos
		return consumed, rl
	}
	return consumed, linkNode
}

func (d *Document) parseTimestamp(input string, start int) (int, Node) {
	return d.parseTimestampWithPos(input, start, 0, 0)
}

func (d *Document) parseTimestampWithPos(input string, start int, startLine, startColumn int) (int, Node) {
	if m := timestampRegexp.FindStringSubmatch(input[start:]); m != nil {
		ddmmyy, hhmm, interval, isDate := m[1], m[3], strings.TrimSpace(m[4]), false
		if hhmm == "" {
			hhmm, isDate = "00:00", true
		}
		t, err := time.Parse(timestampFormat, fmt.Sprintf("%s Mon %s", ddmmyy, hhmm))
		if err != nil {
			return 0, nil
		}
		consumed := len(m[0])
		pos := positionFromChars(input, startLine, startColumn, start, start+consumed)
		timestamp := Timestamp{Time: t, IsDate: isDate, Interval: interval, Pos: pos}
		return consumed, timestamp
	}
	return 0, nil
}

func (d *Document) parseEmphasis(input string, start int, isRaw bool) (int, Node) {
	return d.parseEmphasisWithPos(input, start, isRaw, 0, 0)
}

func (d *Document) parseEmphasisWithPos(input string, start int, isRaw bool, startLine, startColumn int) (int, Node) {
	marker, i := input[start], start
	if !hasValidPreAndBorderChars(input, i) {
		return 0, nil
	}
	for i, consumedNewLines := i+1, 0; i < len(input) && consumedNewLines <= d.MaxEmphasisNewLines; i++ {
		if input[i] == '\n' {
			consumedNewLines++
		}

		if input[i] == marker && i != start+1 && hasValidPostAndBorderChars(input, i) {
			var content []Node
			if isRaw {
				content = d.parseRawInline(input[start+1 : i])
			} else {
				content = d.parseInlineWithPos(input[start+1:i], startLine, startColumn+start+1)
			}
			pos := positionFromChars(input, startLine, startColumn, start, i+1)
			return i + 1 - start, Emphasis{Kind: input[start : start+1], Content: content, Pos: pos}
		}
	}
	return 0, nil
}

// see org-emphasis-regexp-components (emacs elisp variable)

func hasValidPreAndBorderChars(input string, i int) bool {
	return isValidBorderChar(nextRune(input, i)) && isValidPreChar(prevRune(input, i))
}

func hasValidPostAndBorderChars(input string, i int) bool {
	return (isValidPostChar(nextRune(input, i))) && isValidBorderChar(prevRune(input, i))
}

func prevRune(input string, i int) rune {
	r, _ := utf8.DecodeLastRuneInString(input[:i])
	return r
}

func nextRune(input string, i int) rune {
	_, c := utf8.DecodeRuneInString(input[i:])
	r, _ := utf8.DecodeRuneInString(input[i+c:])
	return r
}

func isValidPreChar(r rune) bool {
	return r == utf8.RuneError || unicode.IsSpace(r) || strings.ContainsRune(`-({'"`, r)
}

func isValidPostChar(r rune) bool {
	return r == utf8.RuneError || unicode.IsSpace(r) || strings.ContainsRune(`-.,:!?;'")}[\`, r)
}

func isValidBorderChar(r rune) bool { return !unicode.IsSpace(r) }

func (l RegularLink) Kind() string {
	description := String(l.Description...)
	descProtocol, descExt := strings.SplitN(description, ":", 2)[0], path.Ext(description)
	if ok := descProtocol == "file" || descProtocol == "http" || descProtocol == "https"; ok && imageExtensionRegexp.MatchString(descExt) {
		return "image"
	} else if ok && videoExtensionRegexp.MatchString(descExt) {
		return "video"
	}

	if p := l.Protocol; l.Description != nil || (p != "" && p != "file" && p != "http" && p != "https") {
		return "regular"
	}
	if imageExtensionRegexp.MatchString(path.Ext(l.URL)) {
		return "image"
	}
	if videoExtensionRegexp.MatchString(path.Ext(l.URL)) {
		return "video"
	}
	return "regular"
}

func (n Text) String() string              { return String(n) }
func (n LineBreak) String() string         { return String(n) }
func (n ExplicitLineBreak) String() string { return String(n) }
func (n StatisticToken) String() string    { return String(n) }
func (n Emphasis) String() string          { return String(n) }
func (n InlineBlock) String() string       { return String(n) }
func (n LatexFragment) String() string     { return String(n) }
func (n FootnoteLink) String() string      { return String(n) }
func (n RegularLink) String() string       { return String(n) }
func (n Macro) String() string             { return String(n) }
func (n Timestamp) String() string         { return String(n) }
