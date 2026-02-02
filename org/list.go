package org

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type ListKind int

const (
	UnorderedList ListKind = iota
	OrderedList
	DescriptiveList
)

func (k ListKind) String() string {
	switch k {
	case UnorderedList:
		return "unordered"
	case OrderedList:
		return "ordered"
	case DescriptiveList:
		return "descriptive"
	default:
		return "unknown"
	}
}

type List struct {
	Kind  ListKind
	Items []Node
	Pos   Position
}

type ListItem struct {
	Bullet   string
	Status   string
	Value    string
	Children []Node
	Pos      Position
}

type DescriptiveListItem struct {
	Bullet  string
	Status  string
	Term    []Node
	Details []Node
	Pos     Position
}

var unorderedListRegexp = regexp.MustCompile(`^(\s*)([+*-])(\s+(.*)|$)`)
var orderedListRegexp = regexp.MustCompile(`^(\s*)(([0-9]+|[a-zA-Z])[.)])(\s+(.*)|$)`)
var descriptiveListItemRegexp = regexp.MustCompile(`\s::(\s|$)`)
var listItemValueRegexp = regexp.MustCompile(`\[@(\d+)\]\s`)
var listItemStatusRegexp = regexp.MustCompile(`\[( |X|-)\]\s`)

func lexList(line string) (token, bool) {
	if m := unorderedListRegexp.FindStringSubmatch(line); m != nil {
		return token{kind: "unorderedList", lvl: len(m[1]), content: m[4], matches: m}, true
	} else if m := orderedListRegexp.FindStringSubmatch(line); m != nil {
		return token{kind: "orderedList", lvl: len(m[1]), content: m[5], matches: m}, true
	}
	return nilToken, false
}

func isListToken(t token) bool {
	return t.kind == "unorderedList" || t.kind == "orderedList"
}

func listKind(t token) (ListKind, ListKind) {
	mainKind := UnorderedList
	switch bullet := t.matches[2]; {
	case bullet == "*" || bullet == "+" || bullet == "-":
		mainKind = UnorderedList
	case unicode.IsLetter(rune(bullet[0])), unicode.IsDigit(rune(bullet[0])):
		mainKind = OrderedList
	default:
		panic(fmt.Sprintf("bad list bullet '%s': %#v", bullet, t))
	}
	if descriptiveListItemRegexp.MatchString(t.content) {
		return mainKind, DescriptiveList
	}
	return mainKind, mainKind
}

func (d *Document) parseList(i int, parentStop stopFn) (int, Node) {
	start, lvl := i, d.tokens[i].lvl
	listMainKind, kind := listKind(d.tokens[i])
	list := List{Kind: kind}
	stop := func(*Document, int) bool {
		if parentStop(d, i) || d.tokens[i].lvl != lvl || !isListToken(d.tokens[i]) {
			return true
		}
		itemMainKind, _ := listKind(d.tokens[i])
		return itemMainKind != listMainKind
	}
	for !stop(d, i) {
		consumed, node := d.parseListItem(list, i, parentStop)
		i += consumed
		list.Items = append(list.Items, node)
	}
	list.Pos = Position{
		StartLine:   d.tokens[start].line,
		StartColumn: d.tokens[start].startCol,
		EndLine:     d.tokens[i-1].line,
		EndColumn:   d.tokens[i-1].endCol,
	}
	return i - start, list
}

func (d *Document) parseListItem(l List, i int, parentStop stopFn) (int, Node) {
	start, nodes, bullet := i, []Node{}, d.tokens[i].matches[2]
	minIndent, dterm, content, status, value := d.tokens[i].lvl+len(bullet), "", d.tokens[i].content, "", ""
	originalBaseLvl := d.baseLvl
	d.baseLvl = minIndent + 1
	if m := listItemValueRegexp.FindStringSubmatch(content); m != nil && l.Kind == OrderedList {
		value, content = m[1], content[len("[@] ")+len(m[1]):]
	}
	if m := listItemStatusRegexp.FindStringSubmatch(content); m != nil {
		status, content = m[1], content[len("[ ] "):]
	}
	if l.Kind == DescriptiveList {
		if m := descriptiveListItemRegexp.FindStringIndex(content); m != nil {
			dterm, content = content[:m[0]], content[m[1]:]
			d.baseLvl = strings.Index(d.tokens[i].matches[0], " ::") + 4
		}
	}

	var ok bool
	d.tokens[i], ok = tokenize(strings.Repeat(" ", minIndent) + content)
	if !ok {
		line := d.tokens[i].line
		d.AddError(ErrorTypeTokenization, "could not lex line", getPositionFromToken(d.tokens[i]), d.tokens[i], fmt.Errorf("no lexer matched: %q", line))
	}
	stop := func(d *Document, i int) bool {
		if parentStop(d, i) {
			return true
		}
		t := d.tokens[i]
		return t.lvl < minIndent && !(t.kind == "text" && t.content == "")
	}
	for !stop(d, i) && (i <= start+1 || !isSecondBlankLine(d, i)) {
		consumed, node := d.parseOne(i, stop)
		i += consumed
		nodes = append(nodes, node)
	}
	d.baseLvl = originalBaseLvl
	if l.Kind == DescriptiveList {
		item := DescriptiveListItem{Bullet: bullet, Status: status, Term: d.parseInline(dterm), Details: nodes}
		item.Pos = Position{
			StartLine:   d.tokens[start].line,
			StartColumn: d.tokens[start].startCol,
			EndLine:     d.tokens[i-1].line,
			EndColumn:   d.tokens[i-1].endCol,
		}
		return i - start, item
	}
	item := ListItem{Bullet: bullet, Status: status, Value: value, Children: nodes}
	item.Pos = Position{
		StartLine:   d.tokens[start].line,
		StartColumn: d.tokens[start].startCol,
		EndLine:     d.tokens[i-1].line,
		EndColumn:   d.tokens[i-1].endCol,
	}
	return i - start, item
}

func (n List) String() string                { return String(n) }
func (n ListItem) String() string            { return String(n) }
func (n DescriptiveListItem) String() string { return String(n) }

func (n List) Copy() Node {
	return List{
		Kind:  n.Kind,
		Items: CopyNodes(n.Items),
		Pos:   n.Pos,
	}
}

func (n ListItem) Copy() Node {
	return ListItem{
		Bullet:   n.Bullet,
		Status:   n.Status,
		Value:    n.Value,
		Children: CopyNodes(n.Children),
		Pos:      n.Pos,
	}
}

func (n DescriptiveListItem) Copy() Node {
	return DescriptiveListItem{
		Bullet:  n.Bullet,
		Status:  n.Status,
		Term:    CopyNodes(n.Term),
		Details: CopyNodes(n.Details),
		Pos:     n.Pos,
	}
}

func (n List) Range(f func(Node) bool) {
	for _, child := range n.Items {
		if !f(child) {
			return
		}
	}
}

func (n List) Position() Position { return n.Pos }

func (n ListItem) Range(f func(Node) bool) {
	for _, child := range n.Children {
		if !f(child) {
			return
		}
	}
}

func (n ListItem) Position() Position { return n.Pos }

func (n DescriptiveListItem) Range(f func(Node) bool) {
	for _, child := range n.Term {
		if !f(child) {
			return
		}
	}
	for _, child := range n.Details {
		if !f(child) {
			return
		}
	}
}

func (n DescriptiveListItem) Position() Position { return n.Pos }
