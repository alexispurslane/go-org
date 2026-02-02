package org

import (
	"fmt"
	"regexp"
)

type FootnoteDefinition struct {
	Name     string
	Children []Node
	Inline   bool
	Pos      Position
}

var footnoteDefinitionRegexp = regexp.MustCompile(`^\[fn:([\w-]+)\](\s+(.+)|\s*$)`)

func lexFootnoteDefinition(line string) (token, bool) {
	if m := footnoteDefinitionRegexp.FindStringSubmatch(line); m != nil {
		return token{kind: "footnoteDefinition", lvl: 0, content: m[1], matches: m}, true
	}
	return nilToken, false
}

func (d *Document) parseFootnoteDefinition(i int, parentStop stopFn) (int, Node) {
	start, name := i, d.tokens[i].content
	startToken := d.tokens[start]
	var ok bool
	d.tokens[i], ok = tokenize(d.tokens[i].matches[2])
	if !ok {
		line := d.tokens[i].line
		d.AddError(ErrorTypeTokenization, "could not lex line", getPositionFromToken(d.tokens[i]), d.tokens[i], fmt.Errorf("no lexer matched: %q", line))
	}
	stop := func(d *Document, i int) bool {
		return parentStop(d, i) ||
			(isSecondBlankLine(d, i) && i > start+1) ||
			d.tokens[i].kind == "headline" || d.tokens[i].kind == "footnoteDefinition"
	}
	consumed, nodes := d.parseMany(i, stop)
	definition := FootnoteDefinition{Name: name, Children: nodes, Inline: false}
	if consumed > 0 {
		definition.Pos = Position{
			StartLine:   startToken.line,
			StartColumn: startToken.startCol,
			EndLine:     d.tokens[start+consumed-1].line,
			EndColumn:   d.tokens[start+consumed-1].endCol,
		}
	}
	return consumed, definition
}

func (n FootnoteDefinition) String() string { return String(n) }

func (n FootnoteDefinition) Copy() Node {
	return FootnoteDefinition{
		Name:     n.Name,
		Children: CopyNodes(n.Children),
		Inline:   n.Inline,
		Pos:      n.Pos,
	}
}

func (n FootnoteDefinition) Range(f func(Node) bool) {
	for _, child := range n.Children {
		if !f(child) {
			return
		}
	}
}

func (n FootnoteDefinition) Position() Position { return n.Pos }
