package mcp

import (
	"testing"

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
	}
	for name, uri := range want {
		if bound[name] != uri {
			t.Errorf("tool %s bound to %q, want %q", name, bound[name], uri)
		}
	}
}
