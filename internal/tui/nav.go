package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
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
	kind       NavItemKind
	specIdx    int
	specTitle  string
	specSource string
	folderPath string
	collapsed  bool
	ep         *EndpointItem
}

var (
	specHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	folderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
)

func (n NavItem) Title() string {
	switch n.kind {
	case NavKindSpec:
		arrow := "▼"
		if n.collapsed {
			arrow = "▶"
		}
		return specHeaderStyle.Render(arrow+" "+n.specTitle) + dimStyle.Render("  x to remove")
	case NavKindFolder:
		arrow := "▼"
		if n.collapsed {
			arrow = "▶"
		}
		return "  " + folderStyle.Render(arrow+" "+n.folderPath)
	case NavKindEndpoint:
		return "    " + n.ep.Title()
	}
	return ""
}

func (n NavItem) Description() string {
	if n.kind == NavKindEndpoint && n.ep.Summary != "" {
		return "      " + dimStyle.Render(n.ep.Summary)
	}
	if n.kind == NavKindSpec {
		return "    " + dimStyle.Render(n.specSource)
	}
	return ""
}

func (n NavItem) FilterValue() string {
	switch n.kind {
	case NavKindEndpoint:
		return n.ep.FilterValue()
	case NavKindFolder:
		return n.folderPath
	default:
		return n.specTitle
	}
}

// buildNavItems constructs the full (unfiltered) tree from all loaded specs.
func buildNavItems(specs []*spec.LoadedSpec) []NavItem {
	var all []NavItem
	for i, s := range specs {
		all = append(all, NavItem{
			kind:       NavKindSpec,
			specIdx:    i,
			specTitle:  s.Title,
			specSource: s.Source,
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
