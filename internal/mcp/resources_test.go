package mcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Every tool that advertises an MCP-app UI must point at a resource the server
// actually serves, and must advertise an output schema (without one, hosts
// drop structuredContent and the app hangs on "Loading…" — see bindUI).
func TestToolUIBindings(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	RegisterTools(s, newFakeFlow(t).client())
	RegisterResources(s)
	resources := s.ListResources()

	bound := map[string]string{}
	for name, st := range s.ListTools() {
		if st.Tool.Meta == nil {
			continue
		}
		ui, _ := st.Tool.Meta.AdditionalFields["ui"].(map[string]any)
		uri, _ := ui["resourceUri"].(string)
		if uri == "" {
			continue
		}
		bound[name] = uri
		if _, ok := resources[uri]; !ok {
			t.Errorf("tool %s is bound to %s, which is not a registered resource", name, uri)
		}
		if len(st.Tool.RawOutputSchema) == 0 {
			t.Errorf("tool %s is bound to a UI but advertises no output schema", name)
		}
	}
	want := map[string]string{
		"list_favorites":          favoritesUI,
		"get_favorite":            favoritesUI,
		"create_favorite":         favoritesUI,
		"update_favorite":         favoritesUI,
		"rename_favorite":         favoritesUI,
		"set_favorite_sport":      favoritesUI,
		"save_target_as_favorite": favoritesUI,
		"schedule_favorite":       favoritesUI,
		"edit_training_session":   "ui://polar-flow/sessions.html",
		"get_training_zones":      "ui://polar-flow/zones.html",
	}
	for name, uri := range want {
		if bound[name] != uri {
			t.Errorf("tool %s bound to %q, want %q", name, bound[name], uri)
		}
	}
}

// Every MCP-app UI must route tool results through the shared notice
// dispatcher. A UI that renders payloads directly stays on "Loading…" when a
// call is rejected, answers with a plain statement ("No target with id 7."),
// is declined by the user, or never delivers a result. Checked against the
// resources the server serves, so a newly added UI is covered automatically.
func TestUIsUseNoticeDispatcher(t *testing.T) {
	s := server.NewMCPServer("test", "0")
	RegisterResources(s)
	resources := s.ListResources()
	if len(resources) == 0 {
		t.Fatal("no UI resources registered")
	}
	markers := map[string]string{
		"tool results go through dispatch":  `m.method === "ui/notifications/tool-result") dispatch(m.params,`,
		"cancelled calls render a notice":   `m.method === "ui/notifications/tool-cancelled")`,
		"errors and notices are classified": `function noticeFor(params)`,
		"dispatcher is defined":             `function dispatch(params, view)`,
		"silence times out into a notice":   `if (!gotResult) renderNotice(`,
	}
	for uri, r := range resources {
		contents, err := r.Handler(context.Background(), mcpgo.ReadResourceRequest{
			Params: mcpgo.ReadResourceParams{URI: uri},
		})
		if err != nil || len(contents) != 1 {
			t.Fatalf("%s: read = %v, %v", uri, contents, err)
		}
		tc, ok := contents[0].(mcpgo.TextResourceContents)
		if !ok {
			t.Fatalf("%s: contents are %T, want text", uri, contents[0])
		}
		for what, marker := range markers {
			if !strings.Contains(tc.Text, marker) {
				t.Errorf("%s: %s — missing %q (copy the notice block from user.html)", uri, what, marker)
			}
		}
		if strings.Contains(tc.Text, "render(extractPayload(m.params))") {
			t.Errorf("%s: renders tool results directly, bypassing dispatch", uri)
		}
	}
}
