package org

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func isSecondBlankLine(d *Document, i int) bool {
	if i-1 <= 0 {
		return false
	}
	t1, t2 := d.tokens[i-1], d.tokens[i]
	if t1.kind == "text" && t2.kind == "text" && t1.content == "" && t2.content == "" {
		return true
	}
	return false
}

func isImageOrVideoLink(n Node) bool {
	if l, ok := n.(RegularLink); ok && l.Kind() == "video" || l.Kind() == "image" {
		return true
	}
	return false
}

// Parse ranges like this:
// "3-5" -> [[3, 5]]
// "3 8-10" -> [[3, 3], [8, 10]]
// "3  5 6" -> [[3, 3], [5, 5], [6, 6]]
//
// This is Hugo's hlLinesToRanges with "startLine" removed and errors
// ignored.
func ParseRanges(s string) [][2]int {
	var ranges [][2]int
	s = strings.TrimSpace(s)
	if s == "" {
		return ranges
	}
	fields := strings.SplitSeq(s, " ")
	for field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		numbers := strings.Split(field, "-")
		var r [2]int
		if len(numbers) > 1 {
			first, err := strconv.Atoi(numbers[0])
			if err != nil {
				return ranges
			}
			second, err := strconv.Atoi(numbers[1])
			if err != nil {
				return ranges
			}
			r[0] = first
			r[1] = second
		} else {
			first, err := strconv.Atoi(numbers[0])
			if err != nil {
				return ranges
			}
			r[0] = first
			r[1] = first
		}

		ranges = append(ranges, r)
	}
	return ranges
}

func IsNewLineChar(r rune) bool {
	return r == '\n' || r == '\r'
}

// PrintNodeTree returns a string representation of an org.Node hierarchy as a tree showing types, positions, and string representations.
func PrintNodeTree(nodes []Node, indent string) string {
	var builder strings.Builder
	for _, node := range nodes {
		if node == nil {
			fmt.Fprintf(&builder, "%s<nil>\n", indent)
			continue
		}

		// Get type information via reflection
		nodeType := reflect.TypeOf(node)
		if nodeType.Kind() == reflect.Pointer {
			nodeType = nodeType.Elem()
		}

		// Get position
		pos := node.Position()
		posStr := fmt.Sprintf("%d:%d-%d:%d", pos.StartLine, pos.StartColumn, pos.EndLine, pos.EndColumn)

		// Get string representation
		nodeStr := String(node)
		// Truncate if too long
		if len(nodeStr) > 100 {
			nodeStr = nodeStr[:97] + "..."
		}
		// Escape newlines and tabs for cleaner display
		nodeStr = strings.ReplaceAll(nodeStr, "\n", `\n`)
		nodeStr = strings.ReplaceAll(nodeStr, "\t", `\t`)

		// Print current node
		fmt.Fprintf(&builder, "%s%s [%s] %q\n", indent, nodeType.Name(), posStr, nodeStr)

		// Recursively print children
		node.Range(func(child Node) bool {
			builder.WriteString(PrintNodeTree([]Node{child}, indent+"  "))
			return true
		})
	}
	return builder.String()
}
