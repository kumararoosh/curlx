package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arooshkumar/curlx/internal/auth"
	"github.com/arooshkumar/curlx/internal/cache"
	"github.com/arooshkumar/curlx/internal/config"
	"github.com/arooshkumar/curlx/internal/spec"
)

// pane identifies which of the three main panels has focus.
type pane int

const (
	paneEndpoints pane = iota
	paneRequest
	paneResponse
)

// appMode controls whether a modal overlay is active.
type appMode int

const (
	modeNormal    appMode = iota
	modeLoadSpec          // l — enter a spec path or git URL
	modeAuthSwitch        // a — pick an auth context
	modeBodyCache         // b — pick a saved request body
	modeNewAuth           // A — add a new auth context
)

// --- Styles ---

var (
	focusedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))

	dimBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62")).
			Padding(0, 1)

	labelStyle = lipgloss.NewStyle().Bold(true)

	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	methodColors = map[string]lipgloss.Color{
		"GET":    "42",
		"POST":   "214",
		"PUT":    "33",
		"PATCH":  "178",
		"DELETE": "196",
	}

	statusOK  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	statusErr = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// App is the root Bubble Tea model.
type App struct {
	mode       appMode
	activePane pane
	width      int
	height     int

	// Main panes
	endpointList list.Model
	urlInput     textinput.Model
	bodyInput    textinput.Model
	activeInput  int  // 0 = url, 1 = body
	inputFocused bool // true = typing mode; global shortcuts disabled
	responseView viewport.Model
	responseBody string

	// Loaded specs and nav tree
	loadedSpecs  []*spec.LoadedSpec
	allNavItems  []NavItem

	// Persistent config
	cfg *config.Config

	// Overlay — spec loader
	specPathInput textinput.Model

	// Overlay — auth switcher
	authCtxList list.Model
	activeAuth  *auth.Context
	authStore   *auth.Store

	// Overlay — new auth form
	newAuthInputs  []textinput.Model
	newAuthFocused int

	// Overlay — body cache
	bodyCacheList list.Model
	bodyCache     *cache.Cache

	// Mouse
	mouseEnabled bool

	// Status bar
	statusMsg  string
	statusIsOk bool
}

// NewApp creates an App, wiring the auth store and body cache.
func NewApp() App {
	// URL input
	ul := textinput.New()
	ul.Placeholder = "https://api.example.com/v1/endpoint"
	ul.Focus()

	// Body input
	bl := textinput.New()
	bl.Placeholder = `{"key": "value"}`

	// Response viewport
	rv := viewport.New(0, 0)

	// Endpoint list
	delegate := list.NewDefaultDelegate()
	el := list.New([]list.Item{}, delegate, 0, 0)
	el.Title = "Endpoints"
	el.SetShowStatusBar(false)
	el.SetFilteringEnabled(true)

	// Spec path overlay input
	si := textinput.New()
	si.Placeholder = "/path/to/openapi.yaml  or  https://github.com/org/repo/.../spec.yaml"

	// Auth context list overlay
	al := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	al.Title = "Auth Contexts  (enter to select · A to add)"
	al.SetShowStatusBar(false)
	al.KeyMap.Quit.SetEnabled(false)

	// New auth form inputs: name, type, token, header key
	newAuthFields := []string{"Name", "Type (bearer|apikey|basic)", "Token", "Header Key (apikey only)"}
	newAuthInputs := make([]textinput.Model, len(newAuthFields))
	for i, ph := range newAuthFields {
		t := textinput.New()
		t.Placeholder = ph
		newAuthInputs[i] = t
	}
	newAuthInputs[0].Focus()

	// Body cache list overlay
	bc := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	bc.Title = "Saved Bodies  (enter to load · s to save current)"
	bc.SetShowStatusBar(false)
	bc.KeyMap.Quit.SetEnabled(false)

	// Auth store (graceful degradation if unavailable)
	store, err := auth.NewStore(machinePassphrase())
	if err != nil {
		store = nil
	}

	// Body cache (graceful degradation)
	bodyCache, err := cache.Open()
	if err != nil {
		bodyCache = nil
	}

	// Persistent config
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	return App{
		mode:          modeNormal,
		activePane:    paneEndpoints,
		endpointList:  el,
		urlInput:      ul,
		bodyInput:     bl,
		responseView:  rv,
		specPathInput: si,
		authCtxList:   al,
		newAuthInputs: newAuthInputs,
		bodyCacheList: bc,
		authStore:     store,
		bodyCache:     bodyCache,
		cfg:           cfg,
		mouseEnabled:  true,
		statusMsg:     "l load spec · a auth · b bodies · Tab switch pane · ctrl+r send · m toggle mouse",
	}
}

func (a App) Init() tea.Cmd {
	if a.cfg == nil || len(a.cfg.SpecSources) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(a.cfg.SpecSources))
	for i, source := range a.cfg.SpecSources {
		cmds[i] = cmdLoadSpec(source)
	}
	return tea.Batch(cmds...)
}

// Update handles all messages.
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a = a.recalculateSizes()

	case responseMsg:
		statusLine := msg.status + "  " + dimStyle.Render(msg.duration)
		a.responseBody = statusLine + "\n\n" + msg.body
		a.responseView.SetContent(a.responseBody)
		a.statusIsOk = strings.HasPrefix(msg.status, "2")
		a.statusMsg = "Response: " + msg.status + " (" + msg.duration + ")"
		a.mode = modeNormal
		a.activePane = paneResponse

	case errMsg:
		a.statusMsg = "Error: " + msg.Error()
		a.statusIsOk = false

	case specLoadedMsg:
		// Replace existing entry for same source, otherwise append.
		replaced := false
		for i, s := range a.loadedSpecs {
			if s.Source == msg.source {
				a.loadedSpecs[i] = msg.spec
				replaced = true
				break
			}
		}
		if !replaced {
			a.loadedSpecs = append(a.loadedSpecs, msg.spec)
			if a.cfg != nil {
				a.cfg.AddSource(msg.source)
				_ = a.cfg.Save()
			}
		}
		a.allNavItems = buildNavItems(a.loadedSpecs)
		a.endpointList.SetItems(visibleNavItems(a.allNavItems))
		a.statusMsg = fmt.Sprintf("Loaded: %s (%d spec(s))", msg.spec.Title, len(a.loadedSpecs))
		a.statusIsOk = true
		a.mode = modeNormal

	case authListMsg:
		a.authCtxList.SetItems(authItems(msg))

	case bodyListMsg:
		a.bodyCacheList.SetItems(bodyItems(msg))

	case clipboardMsg:
		if msg.ok {
			a.statusMsg = "Response copied to clipboard"
			a.statusIsOk = true
		} else {
			a.statusMsg = "Clipboard unavailable"
			a.statusIsOk = false
		}
	}

	// Route key events by mode
	switch a.mode {
	case modeLoadSpec:
		a, cmds = a.updateLoadSpec(msg, cmds)
	case modeAuthSwitch:
		a, cmds = a.updateAuthSwitch(msg, cmds)
	case modeNewAuth:
		a, cmds = a.updateNewAuth(msg, cmds)
	case modeBodyCache:
		a, cmds = a.updateBodyCache(msg, cmds)
	default:
		a, cmds = a.updateNormal(msg, cmds)
	}

	return a, tea.Batch(cmds...)
}

func (a App) updateNormal(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	// When an input is focused, route all keys directly to it.
	// Only esc and ctrl+c escape input mode.
	if a.inputFocused {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return a, append(cmds, tea.Quit)
			case "esc":
				a.inputFocused = false
				a.urlInput.Blur()
				a.bodyInput.Blur()
				a.statusMsg = "Command mode · tab to switch pane · enter to type · ctrl+r to send"
				return a, cmds
			case "tab":
				// Toggle between URL and body while staying in typing mode.
				a.activeInput = 1 - a.activeInput
				if a.activeInput == 0 {
					a.urlInput.Focus()
					a.bodyInput.Blur()
				} else {
					a.bodyInput.Focus()
					a.urlInput.Blur()
				}
				return a, cmds
			}
		}
		// Forward all other keystrokes to the active input.
		var cmd tea.Cmd
		if a.activeInput == 0 {
			a.urlInput, cmd = a.urlInput.Update(msg)
		} else {
			a.bodyInput, cmd = a.bodyInput.Update(msg)
		}
		return a, append(cmds, cmd)
	}

	// Command mode — single-key shortcuts active.
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return a, append(cmds, tea.Quit)

		case "m":
			a.mouseEnabled = !a.mouseEnabled
			if a.mouseEnabled {
				a.statusMsg = "Mouse enabled · m to disable for text selection"
				a.statusIsOk = true
				return a, append(cmds, tea.EnableMouseCellMotion)
			}
			a.statusMsg = "Mouse disabled · drag to select · m to re-enable"
			a.statusIsOk = false
			return a, append(cmds, tea.DisableMouse)

		case "tab":
			a.activePane = (a.activePane + 1) % 3

		case "l":
			a.mode = modeLoadSpec
			a.specPathInput.SetValue("")
			a.specPathInput.Focus()
			return a, cmds

		case "a":
			a.mode = modeAuthSwitch
			if a.authStore != nil {
				cmds = append(cmds, cmdListAuth(a.authStore))
			}
			return a, cmds

		case "b":
			a.mode = modeBodyCache
			if a.bodyCache != nil {
				cmds = append(cmds, cmdListBodies(a.bodyCache, a.selectedOperationID()))
			}
			return a, cmds

		case "s":
			if a.bodyCache != nil && a.bodyInput.Value() != "" {
				opID := a.selectedOperationID()
				name := fmt.Sprintf("body-%d", len(a.bodyCacheList.Items())+1)
				if err := a.bodyCache.Save(name, opID, a.bodyInput.Value()); err != nil {
					a.statusMsg = "Save failed: " + err.Error()
				} else {
					a.statusMsg = `Body saved as "` + name + `"`
					a.statusIsOk = true
				}
			}

		case "ctrl+r":
			if a.activePane == paneRequest {
				return a, append(cmds, cmdSendRequest(a))
			}

		case "y":
			if a.responseBody != "" {
				return a, append(cmds, cmdCopyToClipboard(a.responseBody))
			}

		case "enter":
			if a.activePane == paneEndpoints {
				if nav, ok := a.endpointList.SelectedItem().(NavItem); ok {
					switch nav.kind {
					case NavKindSpec, NavKindFolder:
						// Toggle collapse.
						a.allNavItems = toggleNavItem(a.allNavItems, nav)
						a.endpointList.SetItems(visibleNavItems(a.allNavItems))
					case NavKindEndpoint:
						a.urlInput.SetValue(nav.ep.URL)
						a.activePane = paneRequest
						a.activeInput = 0
						a.inputFocused = true
						a.urlInput.Focus()
						a.bodyInput.Blur()
						a.statusMsg = "Typing mode · esc to return to commands"
					}
				}
			} else if a.activePane == paneRequest {
				a.inputFocused = true
				a.activeInput = 0
				a.urlInput.Focus()
				a.bodyInput.Blur()
				a.statusMsg = "Typing mode · esc to return to commands"
			}

		case "i":
			if a.activePane == paneRequest {
				a.inputFocused = true
				a.activeInput = 1
				a.bodyInput.Focus()
				a.urlInput.Blur()
				a.statusMsg = "Typing mode · esc to return to commands"
			}

		case "x":
			// Remove the selected spec from the tree.
			if a.activePane == paneEndpoints {
				if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindSpec {
					newSpecs := a.loadedSpecs[:0]
					for _, s := range a.loadedSpecs {
						if s.Source != nav.specSource {
							newSpecs = append(newSpecs, s)
						}
					}
					a.loadedSpecs = newSpecs
					a.allNavItems = buildNavItems(a.loadedSpecs)
					a.endpointList.SetItems(visibleNavItems(a.allNavItems))
					if a.cfg != nil {
						a.cfg.RemoveSource(nav.specSource)
						_ = a.cfg.Save()
					}
					a.statusMsg = "Removed: " + nav.specTitle
				}
			}
		}
	}

	// Delegate navigation to the focused pane (command mode only).
	switch a.activePane {
	case paneEndpoints:
		var cmd tea.Cmd
		a.endpointList, cmd = a.endpointList.Update(msg)
		cmds = append(cmds, cmd)
	case paneResponse:
		var cmd tea.Cmd
		a.responseView, cmd = a.responseView.Update(msg)
		cmds = append(cmds, cmd)
	}

	return a, cmds
}

func (a App) updateLoadSpec(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			a.mode = modeNormal
			return a, cmds
		case "enter":
			path := strings.TrimSpace(a.specPathInput.Value())
			if path != "" {
				cmds = append(cmds, cmdLoadSpec(path))
			}
			a.mode = modeNormal
			return a, cmds
		}
	}
	var cmd tea.Cmd
	a.specPathInput, cmd = a.specPathInput.Update(msg)
	cmds = append(cmds, cmd)
	return a, cmds
}

func (a App) updateAuthSwitch(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			a.mode = modeNormal
			return a, cmds
		case "A":
			a.mode = modeNewAuth
			for i := range a.newAuthInputs {
				a.newAuthInputs[i].SetValue("")
			}
			a.newAuthFocused = 0
			a.newAuthInputs[0].Focus()
			return a, cmds
		case "enter":
			switch item := a.authCtxList.SelectedItem().(type) {
			case noAuthItem:
				a.activeAuth = nil
				a.statusMsg = "Auth cleared"
				a.statusIsOk = true
			case authItem:
				a.activeAuth = &item.ctx
				if a.authStore != nil {
					_ = a.authStore.SetActive(item.ctx.Name)
				}
				a.statusMsg = "Auth: " + item.ctx.Name
				a.statusIsOk = true
			}
			a.mode = modeNormal
			return a, cmds
		case "d":
			if item, ok := a.authCtxList.SelectedItem().(authItem); ok {
				if a.authStore != nil {
					_ = a.authStore.Delete(item.ctx.Name)
					cmds = append(cmds, cmdListAuth(a.authStore))
				}
			}
			return a, cmds
		}
	}
	var cmd tea.Cmd
	a.authCtxList, cmd = a.authCtxList.Update(msg)
	cmds = append(cmds, cmd)
	return a, cmds
}

func (a App) updateNewAuth(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			a.mode = modeAuthSwitch
			return a, cmds
		case "tab", "down":
			a.newAuthInputs[a.newAuthFocused].Blur()
			a.newAuthFocused = (a.newAuthFocused + 1) % len(a.newAuthInputs)
			a.newAuthInputs[a.newAuthFocused].Focus()
			return a, cmds
		case "shift+tab", "up":
			a.newAuthInputs[a.newAuthFocused].Blur()
			a.newAuthFocused = (a.newAuthFocused - 1 + len(a.newAuthInputs)) % len(a.newAuthInputs)
			a.newAuthInputs[a.newAuthFocused].Focus()
			return a, cmds
		case "enter":
			if a.authStore != nil {
				ctx := auth.Context{
					Name:      a.newAuthInputs[0].Value(),
					Type:      auth.TokenType(a.newAuthInputs[1].Value()),
					Token:     a.newAuthInputs[2].Value(),
					HeaderKey: a.newAuthInputs[3].Value(),
				}
				if ctx.Name != "" && ctx.Token != "" {
					_ = a.authStore.Save(ctx)
					cmds = append(cmds, cmdListAuth(a.authStore))
					a.statusMsg = "Auth context saved: " + ctx.Name
					a.statusIsOk = true
				}
			}
			a.mode = modeAuthSwitch
			return a, cmds
		}
	}
	var cmd tea.Cmd
	a.newAuthInputs[a.newAuthFocused], cmd = a.newAuthInputs[a.newAuthFocused].Update(msg)
	cmds = append(cmds, cmd)
	return a, cmds
}

func (a App) updateBodyCache(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			a.mode = modeNormal
			return a, cmds
		case "enter":
			if item, ok := a.bodyCacheList.SelectedItem().(bodyItem); ok {
				a.bodyInput.SetValue(item.body.Content)
				a.statusMsg = "Body loaded: " + item.body.Name
				a.statusIsOk = true
			}
			a.mode = modeNormal
			return a, cmds
		case "p":
			// Promote scoped body to global
			if item, ok := a.bodyCacheList.SelectedItem().(bodyItem); ok {
				if a.bodyCache != nil {
					_ = a.bodyCache.Promote(item.body.ID)
					cmds = append(cmds, cmdListBodies(a.bodyCache, a.selectedOperationID()))
				}
			}
			return a, cmds
		case "d":
			if item, ok := a.bodyCacheList.SelectedItem().(bodyItem); ok {
				if a.bodyCache != nil {
					_ = a.bodyCache.Delete(item.body.ID)
					cmds = append(cmds, cmdListBodies(a.bodyCache, a.selectedOperationID()))
				}
			}
			return a, cmds
		}
	}
	var cmd tea.Cmd
	a.bodyCacheList, cmd = a.bodyCacheList.Update(msg)
	cmds = append(cmds, cmd)
	return a, cmds
}

// View renders the full TUI.
func (a App) View() string {
	if a.width == 0 {
		return "Initializing..."
	}

	base := a.baseView()

	switch a.mode {
	case modeLoadSpec:
		return a.overlayView(base, "Load Spec", a.loadSpecOverlay())
	case modeAuthSwitch:
		return a.overlayView(base, "Auth Contexts", a.authSwitchOverlay())
	case modeNewAuth:
		return a.overlayView(base, "New Auth Context", a.newAuthOverlay())
	case modeBodyCache:
		return a.overlayView(base, "Saved Bodies", a.bodyCacheOverlay())
	}

	return base
}

func (a App) baseView() string {
	leftW := a.width / 3
	rightW := a.width - leftW - 2

	// title=1, statusbar=1 → body area = height-2 lines.
	// Each pane border adds 2 (top+bottom), so inner height = body-2.
	bodyH := a.height - 2
	leftPane := a.paneStyle(paneEndpoints).Width(leftW).Height(bodyH - 2).
		Render(a.endpointList.View())

	reqInnerH := bodyH/2 - 2
	reqPane := a.paneStyle(paneRequest).Width(rightW).Height(reqInnerH).
		Render(a.requestView())

	resInnerH := bodyH - bodyH/2 - 2
	resPane := a.paneStyle(paneResponse).Width(rightW).Height(resInnerH).
		Render(a.responseView.View())

	right := lipgloss.JoinVertical(lipgloss.Left, reqPane, resPane)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right)

	authLabel := dimStyle.Render("no auth")
	if a.activeAuth != nil {
		authLabel = statusOK.Render("auth: " + a.activeAuth.Name)
	}

	statusText := a.statusMsg
	statusStyled := dimStyle.Render(statusText)
	if a.statusIsOk {
		statusStyled = statusOK.Render(statusText)
	}

	statusBar := lipgloss.NewStyle().Padding(0, 1).
		Render(statusStyled + "  " + authLabel)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("curlx"),
		body,
		statusBar,
	)
}

func (a App) requestView() string {
	urlLabel := labelStyle.Render("URL")
	if a.activeInput == 0 && a.activePane == paneRequest {
		urlLabel = statusOK.Render("▶ URL")
	}
	bodyLabel := labelStyle.Render("Body")
	if a.activeInput == 1 && a.activePane == paneRequest {
		bodyLabel = statusOK.Render("▶ Body")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		urlLabel,
		a.urlInput.View(),
		"",
		bodyLabel,
		a.bodyInput.View(),
		"",
		dimStyle.Render("enter edit URL · i edit body · tab toggle URL/body · ctrl+r send · s save body · esc command mode"),
	)
}

func (a App) loadSpecOverlay() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		dimStyle.Render("Enter a local file path or git URL, then press enter"),
		"",
		a.specPathInput.View(),
		"",
		dimStyle.Render("esc to cancel"),
	)
}

func (a App) authSwitchOverlay() string {
	return a.authCtxList.View()
}

func (a App) newAuthOverlay() string {
	fields := []string{"Name", "Type", "Token", "Header Key"}
	lines := []string{dimStyle.Render("Tab/↑↓ to move between fields · enter to save · esc to cancel"), ""}
	for i, f := range a.newAuthInputs {
		label := labelStyle.Render(fields[i])
		if i == a.newAuthFocused {
			label = statusOK.Render("▶ " + fields[i])
		}
		lines = append(lines, label, f.View(), "")
	}
	return strings.Join(lines, "\n")
}

func (a App) bodyCacheOverlay() string {
	return a.bodyCacheList.View()
}

// overlayView centers an overlay box over the base view.
func (a App) overlayView(base, title, content string) string {
	box := overlayStyle.Width(a.width / 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(title),
			"",
			content,
		),
	)
	// Simple placement: render overlay after base (terminal overlay is best-effort in TUI)
	_ = base
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, box)
}

func (a App) paneStyle(p pane) lipgloss.Style {
	if a.activePane == p {
		return focusedBorder
	}
	return dimBorder
}

func (a App) recalculateSizes() App {
	bodyH := a.height - 2
	leftW := a.width/3 - 2
	rightW := a.width - a.width/3 - 4

	a.endpointList.SetSize(leftW, bodyH-4)
	a.responseView.Width = rightW - 2
	a.responseView.Height = bodyH - bodyH/2 - 4
	a.authCtxList.SetSize(a.width/2-6, a.height/2)
	a.bodyCacheList.SetSize(a.width/2-6, a.height/2)
	return a
}

func (a App) selectedOperationID() string {
	if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindEndpoint {
		return nav.ep.OperationID
	}
	return ""
}

// machinePassphrase derives a stable passphrase from the hostname.
// In a future version this could be a user-supplied master password.
func machinePassphrase() string {
	h, err := os.Hostname()
	if err != nil {
		h = "curlx-default"
	}
	return "curlx-" + h
}
