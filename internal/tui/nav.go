package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arooshkumar/curlx/internal/spec"
)

// NavItemKind identifies the role of a nav tree node.
type NavItemKind int

const (
	NavKindSpec     NavItemKind = iota // top-level spec group header
	NavKindFolder                      // path-prefix folder (e.g. /books)
	NavKindEndpoint                    // individual API operation
)

// NavItem is a list.Item that represents a node in the spec tree.
type NavItem struct {
	kind            NavItemKind
	specIdx         int
	specTitle       string
	specSource      string
	baseURL         string
	originalBaseURL string
	folderPath      string
	collapsed       bool
	ep              *EndpointItem
}

var (
	specHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	folderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
)

// Title, Description, and FilterValue satisfy list.Item.
// Rendering is handled by navDelegate; these are used only for filtering.
func (n NavItem) Title() string {
	switch n.kind {
	case NavKindEndpoint:
		return n.ep.Method + " " + n.ep.Path
	case NavKindFolder:
		return n.folderPath
	default:
		return n.specTitle
	}
}
func (n NavItem) Description() string { return "" }
func (n NavItem) FilterValue() string  { return n.Title() }

// --- Custom delegate ---

// navDelegate renders NavItems with explicit truncation and per-item layouts
// so hints never overflow the pane width.
type navDelegate struct{}

func newNavDelegate() navDelegate { return navDelegate{} }

// Height is 2: one line for the main label, one for the detail/hint line.
func (d navDelegate) Height() int  { return 2 }
func (d navDelegate) Spacing() int { return 0 }

func (d navDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d navDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	nav, ok := item.(NavItem)
	if !ok {
		return
	}

	width := m.Width()
	selected := index == m.Index()

	var line1, line2 string

	switch nav.kind {
	case NavKindSpec:
		arrow := "▼"
		if nav.collapsed {
			arrow = "▶"
		}
		label := arrow + " " + nav.specTitle
		if selected {
			line1 = specHeaderStyle.Render(trunc(label, width))
		} else {
			line1 = specHeaderStyle.Copy().UnsetBold().Render(trunc(label, width))
		}

		if selected {
			// Show URL (green if overridden) then commands, all truncated to width.
			urlPart := nav.baseURL
			if nav.baseURL != nav.originalBaseURL {
				urlPart = statusOK.Render(trunc(nav.baseURL, width/2)) +
					dimStyle.Render(" (overridden)")
				line2 = "  " + urlPart + "\n  " + dimStyle.Render(trunc("x remove · u override URL", width-2))
			} else {
				line2 = "  " + dimStyle.Render(trunc(urlPart, width/2)) +
					"  " + dimStyle.Render(trunc("x remove · u override URL", width/2-2))
			}
		} else {
			urlPart := nav.baseURL
			if nav.baseURL != nav.originalBaseURL {
				line2 = "  " + statusOK.Render(trunc(urlPart, width-2)) +
					dimStyle.Render(" (overridden)")
			} else {
				line2 = "  " + dimStyle.Render(trunc(urlPart, width-2))
			}
		}

	case NavKindFolder:
		arrow := "▼"
		if nav.collapsed {
			arrow = "▶"
		}
		line1 = "  " + folderStyle.Render(trunc(arrow+" "+nav.folderPath, width-2))

	case NavKindEndpoint:
		line1 = "    " + nav.ep.Title()
		if nav.ep.Summary != "" {
			line2 = "      " + dimStyle.Render(trunc(nav.ep.Summary, width-6))
		}
	}

	fmt.Fprintf(w, "%s\n%s", line1, line2)
}

// trunc truncates s to maxRunes runes, appending "…" if it was cut.
func trunc(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes == 1 {
		return "…"
	}
	return string(r[:maxRunes-1]) + "…"
}

// buildNavItems constructs the full (unfiltered) tree from all loaded specs.
func buildNavItems(specs []*spec.LoadedSpec) []NavItem {
	var all []NavItem
	for i, s := range specs {
		all = append(all, NavItem{
			kind:            NavKindSpec,
			specIdx:         i,
			specTitle:       s.Title,
			specSource:      s.Source,
			baseURL:         s.BaseURL,
			originalBaseURL: s.OriginalBaseURL,
		})

		// Group endpoints by the first path segment.
		type folderEntry struct {
			path string
			eps  []spec.Endpoint
		}
		seen := map[string]bool{}
		var folders []folderEntry
		folderMap := map[string]*folderEntry{}

		for _, ep := range s.Endpoints {
			f := folderFromPath(ep.Path)
			if !seen[f] {
				seen[f] = true
				folders = append(folders, folderEntry{path: f})
				folderMap[f] = &folders[len(folders)-1]
			}
			folderMap[f].eps = append(folderMap[f].eps, ep)
		}

		for _, folder := range folders {
			all = append(all, NavItem{
				kind:       NavKindFolder,
				specIdx:    i,
				folderPath: folder.path,
			})
			for _, ep := range folder.eps {
				ei := EndpointItem{
					Method:      ep.Method,
					Path:        ep.Path,
					URL:         s.BaseURL + ep.Path,
					Summary:     ep.Summary,
					OperationID: ep.OperationID,
					Params:      ep.Params,
				}
				all = append(all, NavItem{
					kind:    NavKindEndpoint,
					specIdx: i,
					ep:      &ei,
				})
			}
		}
	}
	return all
}

// visibleNavItems filters the full tree down to only visible items,
// respecting collapsed state of spec and folder nodes.
func visibleNavItems(all []NavItem) []list.Item {
	// Collect collapsed state in one pass.
	specCollapsed := map[int]bool{}
	folderCollapsed := map[string]bool{}
	for _, item := range all {
		switch item.kind {
		case NavKindSpec:
			specCollapsed[item.specIdx] = item.collapsed
		case NavKindFolder:
			folderCollapsed[folderKey(item.specIdx, item.folderPath)] = item.collapsed
		}
	}

	var visible []list.Item
	for _, item := range all {
		switch item.kind {
		case NavKindSpec:
			visible = append(visible, item)
		case NavKindFolder:
			if !specCollapsed[item.specIdx] {
				visible = append(visible, item)
			}
		case NavKindEndpoint:
			fk := folderKey(item.specIdx, folderFromPath(item.ep.Path))
			if !specCollapsed[item.specIdx] && !folderCollapsed[fk] {
				visible = append(visible, item)
			}
		}
	}
	return visible
}

// toggleNavItem flips the collapsed state of the spec or folder node that
// matches the given selected item, returning the updated slice.
func toggleNavItem(all []NavItem, selected NavItem) []NavItem {
	for i, item := range all {
		switch selected.kind {
		case NavKindSpec:
			if item.kind == NavKindSpec && item.specIdx == selected.specIdx {
				all[i].collapsed = !all[i].collapsed
				return all
			}
		case NavKindFolder:
			if item.kind == NavKindFolder &&
				item.specIdx == selected.specIdx &&
				item.folderPath == selected.folderPath {
				all[i].collapsed = !all[i].collapsed
				return all
			}
		}
	}
	return all
}

func folderKey(specIdx int, folder string) string {
	return fmt.Sprintf("%d:%s", specIdx, folder)
}

func folderFromPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	return "/" + parts[0]
}
