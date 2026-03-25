# Integration Test Migration Plan

## Overview

This document outlines the migration of `server/integration_test.go` from the current monolithic testify-based test suite to a cleaner testing approach using `testza`, chained setup helpers, and explicit `t.Run` structure.

## Goals

1. **Chained setup**: `GivenFile().GivenOpenFile()` for concise test preparation
2. **Generic When**: `When(method, params)` handles any LSP operation with internal polling
3. **Raw assertions**: Use testza directly on results—no hardcoded assertion DSL
4. **No fixed timeouts**: Polling happens inside `When()`
5. **Test isolation**: Each test runs independently with its own server
6. **Explicit structure**: Use `t.Run` for Given/When/Then phases

## Technology Choices

### Assertion Library: testza

[testza](https://github.com/MarvinJWendt/testza) provides colored diffs and clear failure messages:

```go
testza.AssertEqual(t, expected, actual)
testza.AssertTrue(t, condition)
testza.AssertNotNil(t, result)
```

## Architecture

### LSPTestContext

Central type that manages server lifecycle and provides helper methods:

```go
type LSPTestContext struct {
    t       *testing.T
    conn    jsonrpc2.Conn
    ctx     context.Context
    tempDir string  // Temp directory in /tmp for this test instance
}

// The lspstream package provides LargeBufferStream with 64KB buffer.
// Both server and test client use: lspstream.NewLargeBufferStream(conn)

// NewTestContext creates a temp directory in /tmp, starts the LSP server
// with that directory as root, and returns a context for testing.
// The temp directory is automatically cleaned up by the OS.
func NewTestContext(t *testing.T) *LSPTestContext
func (tc *LSPTestContext) Shutdown()
```

### Given Phase Helpers (Chainable)

Setup methods that return `*LSPTestContext` for chaining:

```go
// GivenFile creates a file in the temp directory.
// The path is relative to the temp directory root.
func (tc *LSPTestContext) GivenFile(path, content string) *LSPTestContext

// GivenOpenFile opens a document in the LSP server.
// The uri should be relative (e.g., "file://test.org").
func (tc *LSPTestContext) GivenOpenFile(uri string) *LSPTestContext

// GivenSaveFile triggers a didSave notification for the document.
func (tc *LSPTestContext) GivenSaveFile(uri string) *LSPTestContext
```

### When Phase

A single generic method for LSP operations with internal polling:

```go
func (tc *LSPTestContext) When[T any](method string, params any) T
```

This method:
1. Polls for readiness if needed (internal implementation detail)
2. Sends the LSP request
3. Returns the typed result (type parameter T is inferred from usage)
4. Fails the test if the call errors or returns wrong type

## Test Structure

Tests use `Given`/`When`/`Then` helpers with chaining for the setup phase:

```go
package integration

import (
	"testing"
	
	"github.com/MarvinJWendt/testza"
	"go.lsp.dev/protocol"
)

func TestFileLinkDefinition(t *testing.T) {
    Given("source and target files", t,
        func(t *testing.T) *LSPTestContext {
            tc := NewTestContext(t)
            tc.GivenFile("target.org", "* Target Heading\nContent here").
               GivenFile("source.org", "* Source\nSee [[file:target.org][the target]]").
               GivenOpenFile("file://source.org")
            return tc
        },
        func(t *testing.T, tc *LSPTestContext) {
            When("requesting definition at link position", t, func(t *testing.T) {
                params := protocol.DefinitionParams{
                    TextDocumentPositionParams: protocol.TextDocumentPositionParams{
                        TextDocument: protocol.TextDocumentIdentifier{
                            URI: "file://source.org",
                        },
                        Position: protocol.Position{Line: 1, Character: 10},
                    },
                }
                loc := tc.When[protocol.Location]("textDocument/definition", params)
                
                Then("returns location to target file", t, func(t *testing.T) {
                    testza.AssertTrue(t, strings.Contains(string(loc.URI), "target.org"))
                    testza.AssertEqual(t, uint32(0), loc.Range.Start.Line)
                })
            })
        },
    )
}
```

### Gherkin-style Test Helpers

These helpers wrap `t.Run` and prefix the description for clearer test output:

```go
// Given runs the setup function, calls t.Parallel(), then runs the test function.
// It handles LSPTestContext lifecycle automatically (including Shutdown).
func Given(name string, t *testing.T, setup func(*testing.T) *LSPTestContext, test func(*testing.T, *LSPTestContext)) bool {
    return t.Run("given "+name, func(t *testing.T) {
        t.Parallel()
        tc := setup(t)
        defer tc.Shutdown()
        test(t, tc)
    })
}

func When(name string, t *testing.T, fn func(*testing.T)) bool {
    return t.Run("when "+name, fn)
}

func Then(name string, t *testing.T, fn func(*testing.T)) bool {
    return t.Run("then "+name, fn)
}
```

### UUID Link Test Example

```go
package integration

import (
	"fmt"
	"testing"
	
	"github.com/MarvinJWendt/testza"
	"go.lsp.dev/protocol"
)

func TestUUIDLinkDefinition(t *testing.T) {
    uuid := "12345678-1234-1234-1234-123456789abc"
    
    Given("target with UUID and source with id link", t,
        func(t *testing.T) *LSPTestContext {
            targetContent := fmt.Sprintf(`* Target
:PROPERTIES:
:ID: %s
:END:
Content`, uuid)
            
            tc := NewTestContext(t)
            tc.GivenFile("target.org", targetContent).
               GivenFile("source.org", fmt.Sprintf("* Source\n[[id:%s][link]]", uuid)).
               GivenSaveFile("file://target.org").  // triggers indexing
               GivenOpenFile("file://source.org")
            return tc
        },
        func(t *testing.T, tc *LSPTestContext) {
            When("requesting definition", t, func(t *testing.T) {
                params := protocol.DefinitionParams{
                    TextDocumentPositionParams: protocol.TextDocumentPositionParams{
                        TextDocument: protocol.TextDocumentIdentifier{
                            URI: "file://source.org",
                        },
                        Position: protocol.Position{Line: 1, Character: 10},
                    },
                }
                loc := tc.When[protocol.Location]("textDocument/definition", params)
                
                Then("resolves to target heading", t, func(t *testing.T) {
                    testza.AssertTrue(t, strings.Contains(string(loc.URI), "target.org"))
                })
            })
        },
    )
}
```

### Backlinks Test Example

```go
package integration

import (
	"fmt"
	"testing"
	
	"github.com/MarvinJWendt/testza"
	"go.lsp.dev/protocol"
)

func TestBacklinks(t *testing.T) {
    uuid := "11111111-1111-1111-1111-111111111111"
    
    Given("target with UUID and multiple sources referencing it", t,
        func(t *testing.T) *LSPTestContext {
            targetContent := fmt.Sprintf(`* Target :tag:
:PROPERTIES:
:ID: %s
:END:`, uuid)
            
            tc := NewTestContext(t)
            tc.GivenFile("target.org", targetContent).
               GivenFile("source1.org", fmt.Sprintf("* S1\n[[id:%s][ref1]]", uuid)).
               GivenFile("source2.org", fmt.Sprintf("* S2\n[[id:%s][ref2]]", uuid)).
               GivenSaveFile("file://target.org").
               GivenSaveFile("file://source1.org").
               GivenSaveFile("file://source2.org").
               GivenOpenFile("file://target.org")
            return tc
        },
        func(t *testing.T, tc *LSPTestContext) {
            When("requesting references from target heading", t, func(t *testing.T) {
                params := protocol.ReferenceParams{
                    TextDocumentPositionParams: protocol.TextDocumentPositionParams{
                        TextDocument: protocol.TextDocumentIdentifier{
                            URI: "file://target.org",
                        },
                        Position: protocol.Position{Line: 0, Character: 5},
                    },
                }
                locs := tc.When[[]protocol.Location]("textDocument/references", params)
                
                Then("returns all backlink locations", t, func(t *testing.T) {
                    testza.AssertEqual(t, 2, len(locs))
                })
            })
        },
    )
}
```

## Synchronization Strategy

The `When` method handles polling internally. For operations that need to wait for indexing (like `textDocument/definition` on UUID links), the implementation polls internally:

```go
func (tc *LSPTestContext) When[T any](method string, params any) T {
    // For methods that require indexed data, poll until ready
    if requiresIndexing(method) {
        tc.pollUntilIndexed(params)
    }
    
    // Perform the LSP call using the shared lspstream connection
    var result T
    err := protocol.Call(tc.ctx, tc.conn, method, params, &result)
    if err != nil {
        tc.t.Fatalf("LSP call %s failed: %v", method, err)
    }
    
    return result
}
```

This eliminates the need for explicit `WaitForIndexed()` calls in tests.

## File Structure

```
integration/
├── lsp_test_context.go          # LSPTestContext with Given/When helpers
└── lsp_test.go                  # Integration tests (replaces old)

lspstream/
└── stream.go                    # Shared LargeBufferStream implementation
```

## Dependencies

```bash
go get github.com/MarvinJWendt/testza
```

## Implementation Plan

### Phase 1: Core Framework
1. Create `integration/` directory with package `integration`
2. Create `integration/lsp_test_context.go` - LSPTestContext with Given/When helpers
3. Add testza to go.mod
4. Ensure imports work from `integration` to `lspstream` and `server`

### Phase 2: Port Critical Tests
Create `integration/lsp_test.go` and port essential tests:
- File link definition
- UUID link definition
- Hover on file links
- Backlinks

Tests import and use only public APIs from `server` package.
Each test gets its own temp directory in /tmp - no shared testdata.
Given automatically calls `t.Parallel()` since Gherkin scenarios are independent.

### Phase 3: Port Remaining Tests
Port all remaining tests to `integration/lsp_test.go`:
- Initialization
- Hover (ID links, non-link)
- Completion
- Symbols
- Code actions
- Shutdown

### Phase 4: Cleanup
1. Delete old `server/integration_test.go`
2. Delete `server/testdata/` directory
3. Verify all tests pass with `go test ./integration/...`

## Migration Checklist

- [ ] Phase 1: Core framework
- [ ] Phase 2: Critical tests
- [ ] Phase 3: All remaining tests
- [ ] Phase 4: Cleanup
- [ ] Update AGENTS.md with new patterns
- [ ] Verify with `just test`
