package org

import (
	"fmt"
	"io"
	"strconv"
)

// ErrorType represents the kind of error that occurred.
type ErrorType string

const (
	ErrorTypeInvalidSyntax    ErrorType = "invalid_syntax"
	ErrorTypeUnexpectedToken  ErrorType = "unexpected_token"
	ErrorTypeInvalidStructure ErrorType = "invalid_structure"
	ErrorTypeDuplicateNode    ErrorType = "duplicate_node"
	ErrorTypeMissingNode      ErrorType = "missing_node"
	ErrorTypeValidation       ErrorType = "validation_error"
	ErrorTypeTokenization     ErrorType = "tokenization_error"
	ErrorTypeIO               ErrorType = "io_error"
)

// ParseError is a structured error with detailed position information.
// It provides precise location tracking for syntax and parsing errors.
type ParseError struct {
	Type    ErrorType
	Message string
	File    string

	// Position information
	StartLine int
	EndLine   int
	StartCol  int
	EndCol    int

	// Additional context
	Token   token  // The problematic token, if applicable
	Context string // Additional context or suggestion

	// Underlying cause
	Cause error
}

// Error implements the error interface with a formatted message.
func (e *ParseError) Error() string {
	location := e.locationString()
	msg := e.Message
	if location != "" {
		msg = location + ": " + msg
	}
	if e.Context != "" {
		msg += " (hint: " + e.Context + ")"
	}
	return msg
}

// Unwrap returns the underlying cause for error chain support.
func (e *ParseError) Unwrap() error {
	return e.Cause
}

// locationString formats the position information for display.
func (e *ParseError) locationString() string {
	var loc string
	if e.File != "" {
		loc = e.File + ":"
	}

	// Format line:col-range or line:col
	if e.StartLine == e.EndLine {
		if e.StartCol == e.EndCol {
			loc += fmt.Sprintf("%d:%d", e.StartLine, e.StartCol)
		} else {
			loc += fmt.Sprintf("%d:%d-%d", e.StartLine, e.StartCol, e.EndCol)
		}
	} else {
		loc += fmt.Sprintf("%d:%d-%d:%d", e.StartLine, e.StartCol, e.EndLine, e.EndCol)
	}

	return loc
}

// String provides a detailed string representation including all fields.
func (e *ParseError) String() string {
	s := fmt.Sprintf("%s (type: %s)", e.Error(), e.Type)
	if e.Cause != nil {
		s += fmt.Sprintf("\n  caused by: %v", e.Cause)
	}
	return s
}

// NewParseError creates a new ParseError from the given components.
func NewParseError(typ ErrorType, message, file string, pos Position, tok token, cause error) *ParseError {
	return &ParseError{
		Type:      typ,
		Message:   message,
		File:      file,
		StartLine: pos.StartLine,
		EndLine:   pos.EndLine,
		StartCol:  pos.StartColumn,
		EndCol:    pos.EndColumn,
		Token:     tok,
		Cause:     cause,
	}
}

// AddError adds a new parsing error to the document with detailed position info.
// This is the preferred method for reporting errors during parsing.
func (d *Document) AddError(typ ErrorType, message string, pos Position, tok token, cause error) {
	if d.Errors == nil {
		d.Errors = make([]*ParseError, 0)
	}

	err := NewParseError(typ, message, d.Path, pos, tok, cause)
	d.Errors = append(d.Errors, err)
}

// HasErrors returns true if the document contains any parsing errors.
func (d *Document) HasErrors() bool {
	return len(d.Errors) > 0
}

// HasFatalError returns true if the document has a fatal error that prevented successful parsing.
func (d *Document) HasFatalError() bool {
	return d.FatalError != nil
}

// AddFatalError sets a fatal error that prevents successful parsing.
// This is used for unrecoverable errors where the parser cannot continue.
func (d *Document) AddFatalError(typ ErrorType, message string, pos Position, tok token, cause error) {
	err := NewParseError(typ, message, d.Path, pos, tok, cause)
	d.FatalError = err
	// Also add to Errors slice for completeness
	if d.Errors == nil {
		d.Errors = make([]*ParseError, 0)
	}
	d.Errors = append(d.Errors, err)
}

// WriteErrors writes all document errors to the provided writer, one per line.
func (d *Document) WriteErrors(w io.Writer) error {
	for _, err := range d.Errors {
		_, writeErr := fmt.Fprintln(w, err.Error())
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}

// ErrorCount returns the number of parsing errors in the document.
func (d *Document) ErrorCount() int {
	return len(d.Errors)
}

// GetErrorByType returns all errors of the specified type.
func (d *Document) GetErrorByType(typ ErrorType) []*ParseError {
	result := make([]*ParseError, 0)
	for _, err := range d.Errors {
		if err.Type == typ {
			result = append(result, err)
		}
	}
	return result
}

// getPositionFromToken extracts a Position from a token.
// This helper ensures consistent Position creation from tokens.
func getPositionFromToken(tok token) Position {
	return Position{
		StartLine:   tok.line,
		StartColumn: tok.startCol,
		EndLine:     tok.line,
		EndColumn:   tok.endCol,
	}
}

// formatRange formats a column range, handling edge cases.
func formatRange(startCol, endCol int) string {
	if startCol == endCol {
		return strconv.Itoa(startCol)
	}
	return fmt.Sprintf("%d-%d", startCol, endCol)
}
