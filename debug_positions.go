package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/alexispurslane/go-org/org"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run debug_positions.go <org-file>")
		os.Exit(1)
	}

	filename := os.Args[1]

	// Read and print file with 0-indexed line numbers
	fmt.Printf("=== File with 0-indexed line numbers: %s ===\n", filename)
	printFileWithLineNumbers(filename)
	fmt.Println()

	// Parse the org file
	fmt.Printf("=== Parsed AST with positions: %s ===\n", filename)
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	reader := bytes.NewReader(content)
	doc := org.New().Parse(reader, filename)

	// Walk the tree and print positions
	printNodeTree(doc.Nodes, 0)
}

func printFileWithLineNumbers(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		fmt.Printf("% 4d: %s\n", i, line)
	}
}

func printNodeTree(nodes []org.Node, indent int) {
	indentStr := strings.Repeat("  ", indent)
	for i, node := range nodes {
		fmt.Printf("%s[%d] ", indentStr, i)
		printNodeInfo(node)
		fmt.Printf(" %s\n", formatPosition(getNodePosition(node)))

		// Recursively print children if node has them
		walkNodeChildren(node, indent+1)
	}
}

func walkNodeChildren(node org.Node, indent int) {
	switch n := node.(type) {
	case org.Headline:
		printNodeTree(n.Children, indent)
	case org.Block:
		printNodeTree(n.Children, indent)
		if n.Result != nil {
			printNodeTree([]org.Node{n.Result}, indent)
		}
	case org.Paragraph:
		printNodeTree(n.Children, indent)
	case org.List:
		for _, item := range n.Items {
			printNodeTree([]org.Node{item}, indent+1)
		}
	case org.ListItem:
		printNodeTree(n.Children, indent)
	case org.Drawer:
		printNodeTree(n.Children, indent)
	case org.PropertyDrawer:
		for _, prop := range n.Properties {
			fmt.Printf("%s  Property: %s = %s\n", strings.Repeat("  ", indent), prop[0], prop[1])
		}
	case org.Example:
		printNodeTree(n.Children, indent)
	case org.LatexBlock:
		printNodeTree(n.Content, indent)
	}
}

func printNodeInfo(node org.Node) {
	switch n := node.(type) {
	case org.Headline:
		fmt.Printf("Headline(Lv%d, %s, Title=%q)", n.Lvl, n.Status, truncate(org.String(n.Title...), 40))
	case org.Paragraph:
		fmt.Printf("Paragraph(%d children)", len(n.Children))
	case org.Block:
		fmt.Printf("Block(%s, %d children)", n.Name, len(n.Children))
	case org.List:
		fmt.Printf("List(%d items)", len(n.Items))
	case org.ListItem:
		fmt.Printf("ListItem(%d children)", len(n.Children))
	case org.Keyword:
		fmt.Printf("Keyword(%s=%s)", n.Key, truncate(n.Value, 30))
	case org.Text:
		fmt.Printf("Text(%q)", truncate(n.Content, 40))
	case org.Emphasis:
		fmt.Printf("Emphasis(%s, %d children)", n.Kind, len(n.Content))
	case org.RegularLink:
		fmt.Printf("RegularLink(%s -> %s)", truncate(org.String(n.Description...), 30), truncate(n.URL, 30))
	case org.LatexFragment:
		fmt.Printf("LatexFragment(%s...)", truncate(n.OpeningPair, 20))
	case org.FootnoteLink:
		fmt.Printf("FootnoteLink(%s)", n.Name)
	case org.Comment:
		fmt.Printf("Comment")
	case org.HorizontalRule:
		fmt.Printf("HorizontalRule")
	case org.Drawer:
		fmt.Printf("Drawer(%s)", n.Name)
	case org.PropertyDrawer:
		fmt.Printf("PropertyDrawer(%d props)", len(n.Properties))
	case org.Example:
		fmt.Printf("Example(%d children)", len(n.Children))
	case org.LatexBlock:
		fmt.Printf("LatexBlock(%d children)", len(n.Content))
	case org.Macro:
		fmt.Printf("Macro(%s)", n.Name)
	case org.LineBreak:
		fmt.Printf("LineBreak(%d)", n.Count)
	case org.ExplicitLineBreak:
		fmt.Printf("ExplicitLineBreak")
	case org.InlineBlock:
		fmt.Printf("InlineBlock(%s)", n.Name)
	case org.Timestamp:
		fmt.Printf("Timestamp(%s)", n.Time)
	case org.StatisticToken:
		fmt.Printf("StatisticToken")
	case org.FootnoteDefinition:
		fmt.Printf("FootnoteDefinition(%s)", n.Name)
	default:
		fmt.Printf("%T", node)
	}
}

func getNodePosition(node org.Node) org.Position {
	switch n := node.(type) {
	case org.Headline:
		return n.Pos
	case org.Paragraph:
		return n.Pos
	case org.Block:
		return n.Pos
	case org.Keyword:
		return n.Pos
	case org.Emphasis:
		return n.Pos
	case org.FootnoteDefinition:
		return n.Pos
	case org.LatexFragment:
		return n.Pos
	case org.Text:
		return n.Pos
	case org.Drawer:
		return n.Pos
	case org.PropertyDrawer:
		return n.Pos
	case org.Example:
		return n.Pos
	case org.LatexBlock:
		return n.Pos
	case org.HorizontalRule:
		return n.Pos
	case org.List:
		return n.Pos
	case org.ListItem:
		return n.Pos
	case org.Macro:
		return n.Pos
	case org.LineBreak:
		return n.Pos
	case org.ExplicitLineBreak:
	case org.RegularLink:
		return n.Pos
	case org.FootnoteLink:
		return n.Pos
	}
	return org.Position{}
}

func formatPosition(pos org.Position) string {
	if pos.StartLine == 0 && pos.StartColumn == 0 && pos.EndLine == 0 && pos.EndColumn == 0 {
		return "[no position]"
	}
	return fmt.Sprintf("[L%d:%d-L%d:%d]", pos.StartLine, pos.StartColumn, pos.EndLine, pos.EndColumn)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
