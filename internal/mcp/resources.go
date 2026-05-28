package mcp

import (
	"context"
	_ "embed"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const appMIME = "text/html;profile=mcp-app"

//go:embed ui/user.html
var userHTML []byte

//go:embed ui/targets.html
var targetsHTML []byte

//go:embed ui/sessions.html
var sessionsHTML []byte

//go:embed ui/calendar.html
var calendarHTML []byte

//go:embed ui/progress.html
var progressHTML []byte

// RegisterResources registers all MCP App UI resources with the server.
func RegisterResources(s *server.MCPServer) {
	addResource(s, "ui://polar-flow/user.html", "User Info UI", userHTML)
	addResource(s, "ui://polar-flow/targets.html", "Training Targets UI", targetsHTML)
	addResource(s, "ui://polar-flow/sessions.html", "Training Sessions UI", sessionsHTML)
	addResource(s, "ui://polar-flow/calendar.html", "Calendar UI", calendarHTML)
	addResource(s, "ui://polar-flow/progress.html", "Progress Summary UI", progressHTML)
}

func addResource(s *server.MCPServer, uri, name string, html []byte) {
	body := string(html)
	s.AddResource(mcpgo.Resource{URI: uri, Name: name, MIMEType: appMIME},
		func(_ context.Context, req mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error) {
			return []mcpgo.ResourceContents{
				mcpgo.TextResourceContents{URI: req.Params.URI, MIMEType: appMIME, Text: body},
			}, nil
		})
}
