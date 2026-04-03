package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arooshkumar/curlx/internal/auth"
	"github.com/arooshkumar/curlx/internal/cache"
	"github.com/arooshkumar/curlx/internal/http"
	"github.com/arooshkumar/curlx/internal/spec"
)

// --- Tea messages ---

type responseMsg struct {
	status   string
	duration string
	body     string
}

type errMsg struct{ err error }

func (e errMsg) Error() string { return e.err.Error() }

type specLoadedMsg struct {
	spec   *spec.LoadedSpec
	source string
}
type authListMsg []auth.Context
type bodyListMsg []cache.Body

// --- EndpointItem ---

// EndpointItem is a list.Item representing a single API endpoint.
type EndpointItem struct {
	Method      string
	Path        string
	URL         string
	Summary     string
	OperationID string
}

func (e EndpointItem) Title() string {
	color, ok := methodColors[e.Method]
	if !ok {
		color = "255"
	}
	badge := lipgloss.NewStyle().
		Foreground(lipgloss.Color(color)).
		Bold(true).
		Render(fmt.Sprintf("%-6s", e.Method))
	return badge + " " + e.Path
}

func (e EndpointItem) Description() string { return e.Summary }
func (e EndpointItem) FilterValue() string  { return e.Method + " " + e.Path }


// --- AuthItem ---

type authItem struct{ ctx auth.Context }

func (a authItem) Title() string {
	return a.ctx.Name
}
func (a authItem) Description() string { return string(a.ctx.Type) }
func (a authItem) FilterValue() string  { return a.ctx.Name }

// noAuthItem is a sentinel list item that clears the active auth context.
type noAuthItem struct{}

func (noAuthItem) Title() string       { return dimStyle.Render("(no auth)") }
func (noAuthItem) Description() string { return "send requests without authentication" }
func (noAuthItem) FilterValue() string { return "no auth" }

func authItems(contexts []auth.Context) []list.Item {
	items := []list.Item{noAuthItem{}}
	for _, c := range contexts {
		items = append(items, authItem{ctx: c})
	}
	return items
}

// --- BodyItem ---

type bodyItem struct{ body cache.Body }

func (b bodyItem) Title() string {
	scope := "global"
	if b.body.OperationID != "" {
		scope = b.body.OperationID
	}
	return fmt.Sprintf("%s (%s)", b.body.Name, scope)
}
func (b bodyItem) Description() string {
	preview := b.body.Content
	if len(preview) > 60 {
		preview = preview[:60] + "…"
	}
	return preview
}
func (b bodyItem) FilterValue() string { return b.body.Name }

func bodyItems(bodies []cache.Body) []list.Item {
	items := make([]list.Item, 0, len(bodies))
	for _, b := range bodies {
		items = append(items, bodyItem{body: b})
	}
	return items
}

// --- Commands ---

func cmdSendRequest(a App) tea.Cmd {
	return func() tea.Msg {
		url := a.urlInput.Value()
		if url == "" {
			return errMsg{fmt.Errorf("URL is empty")}
		}

		method := "GET"
		if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindEndpoint {
			method = nav.ep.Method
		}

		req := http.Request{
			Method: method,
			URL:    url,
			Body:   a.bodyInput.Value(),
		}

		resp, err := http.Do(req, a.activeAuth)
		if err != nil {
			return errMsg{err}
		}

		body := prettyJSON(resp.Body)
		return responseMsg{
			status:   resp.Status,
			duration: resp.Duration.Round(1e6).String(),
			body:     body,
		}
	}
}

func cmdLoadSpec(source string) tea.Cmd {
	return func() tea.Msg {
		var s *spec.LoadedSpec
		var err error
		if isGitURL(source) {
			parts := strings.SplitN(source, " ", 2)
			if len(parts) != 2 {
				return errMsg{fmt.Errorf("git URL format: <repo-url>[#branch] <spec-path>")}
			}
			s, err = spec.FromGit(parts[0], parts[1])
		} else {
			s, err = spec.FromFile(source)
		}
		if err != nil {
			return errMsg{err}
		}
		s.Source = source
		return specLoadedMsg{spec: s, source: source}
	}
}

func cmdListAuth(store *auth.Store) tea.Cmd {
	return func() tea.Msg {
		contexts, err := store.List()
		if err != nil {
			return errMsg{err}
		}
		return authListMsg(contexts)
	}
}

func cmdListBodies(c *cache.Cache, operationID string) tea.Cmd {
	return func() tea.Msg {
		bodies, err := c.List(operationID)
		if err != nil {
			return errMsg{err}
		}
		return bodyListMsg(bodies)
	}
}

// --- Helpers ---

func isGitURL(s string) bool {
	return len(s) > 4 && (s[:5] == "https" || s[:4] == "http") && (s[len(s)-5:] == ".yaml" || s[len(s)-4:] == ".yml" || contains(s, ".git"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}

func prettyJSON(data []byte) string {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(pretty)
}
