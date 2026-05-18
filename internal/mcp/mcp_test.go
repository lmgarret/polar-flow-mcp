package mcp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lm/polar-flow-mcp/internal/auth"
	"github.com/lm/polar-flow-mcp/internal/mcp"
	"github.com/lm/polar-flow-mcp/internal/store"
)

// openTestStore opens an in-memory SQLite store with a unique DSN so each test gets
// a clean database. The caller must close the store via t.Cleanup.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := fmt.Sprintf("file:testdb_%d?mode=memory&cache=shared", time.Now().UnixNano())
	st, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("openTestStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// resultText collects all TextContent text values from a CallToolResult and joins them.
func resultText(result *mcpgo.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, " ")
}

// TestGetUserInfo_Linked verifies that a user who has linked their Polar account
// receives a result containing their proxy identity and Polar user ID.
func TestGetUserInfo_Linked(t *testing.T) {
	st := openTestStore(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "alice")

	if err := st.UpsertUser(ctx, "alice", "12345"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	handler := mcp.GetUserInfoHandler(st)
	result, err := handler(ctx, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got error result with text: %s", resultText(result))
	}

	text := resultText(result)
	if !strings.Contains(text, "alice") {
		t.Errorf("result text %q does not contain identity %q", text, "alice")
	}
	if !strings.Contains(strings.ToLower(text), "linked") {
		t.Errorf("result text %q does not contain word 'linked'", text)
	}
}

// TestGetUserInfo_NotLinked verifies that an unlinked user receives a result
// containing their identity and a hint pointing to /oauth/login.
func TestGetUserInfo_NotLinked(t *testing.T) {
	st := openTestStore(t)
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "bob")

	handler := mcp.GetUserInfoHandler(st)
	result, err := handler(ctx, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
		return
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got error result with text: %s", resultText(result))
	}

	text := resultText(result)
	if !strings.Contains(text, "bob") {
		t.Errorf("result text %q does not contain identity %q", text, "bob")
	}
	if !strings.Contains(text, "/oauth/login") {
		t.Errorf("result text %q does not contain /oauth/login hint", text)
	}
	if !strings.Contains(strings.ToLower(text), "no polar account") {
		t.Errorf("result text %q does not contain 'no polar account'", text)
	}
}

// TestGetUserInfo_NoIdentity verifies that when no identity is in context (auth
// middleware not applied), the handler returns an error result mentioning "identity".
func TestGetUserInfo_NoIdentity(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background() // no identity

	handler := mcp.GetUserInfoHandler(st)
	result, err := handler(ctx, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
		return
	}
	if !result.IsError {
		t.Fatalf("expected error result for missing identity, got non-error result: %s", resultText(result))
	}

	text := resultText(result)
	if !strings.Contains(strings.ToLower(text), "identity") {
		t.Errorf("error result text %q does not contain 'identity'", text)
	}
}
