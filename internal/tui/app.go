package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/arooshkumar/curlx/internal/auth"
	"github.com/arooshkumar/curlx/internal/cache"
	"github.com/arooshkumar/curlx/internal/config"
	"github.com/arooshkumar/curlx/internal/spec"
)

// pane identifies which panel has focus.
type pane int

const (
	paneEndpoints pane = iota
	paneRequest          // URL + params table
	paneBody             // body textarea
	paneResponse
)

// appMode controls whether a modal overlay is active.
type appMode int

const (
	modeNormal        appMode = iota
	modeLoadSpec              // l
	modeAuthSwitch            // a
	modeBodyCache             // b
	modeNewAuth               // A
	modeBaseURLOverride       // u
)

// paramRow is one editable row in the params table.
type paramRow struct {
	key      string          // display label for spec-defined rows
	paramIn  string          // "path", "query", or "" for custom
	keyInput textinput.Model // editable key for custom (non-spec) rows
	value    textinput.Model
	fromSpec bool
}

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
	endpointList  list.Model
	urlInput      textinput.Model
	paramRows     []paramRow
	paramFocused  int
	paramSubFocus int  // 0=key (custom rows), 1=value
	bodyInput     textarea.Model
	activeInput   int  // 0=url, 1=params (within paneRequest)
	inputFocused  bool // true = typing mode
	responseView  viewer
	responseBody  string

	// Loaded specs and nav tree
	loadedSpecs []*spec.LoadedSpec
	allNavItems []NavItem

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

	// Overlay — base URL override
	baseURLInput       textinput.Model
	overrideTargetIdx  int // index into loadedSpecs

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

	// Body textarea
	ta := textarea.New()
	ta.Placeholder = `{"key": "value"}`
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.SetWidth(50)
	ta.SetHeight(6)

	// Response viewer
	rv := newViewer(0, 0)

	// Endpoint list
	el := list.New([]list.Item{}, newNavDelegate(), 0, 0)
	el.Title = "Endpoints"
	el.SetShowStatusBar(false)
	el.SetFilteringEnabled(true)

	// Spec path overlay input
	si := textinput.New()
	si.Placeholder = "/path/to/openapi.yaml  or  https://github.com/org/repo/.../spec.yaml"

	// Base URL override input
	bu := textinput.New()
	bu.Placeholder = "https://my-server:8080"

	// Auth context list overlay
	al := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	al.Title = "Auth Contexts  (enter to select · A to add)"
	al.SetShowStatusBar(false)
	al.KeyMap.Quit.SetEnabled(false)

	// New auth form inputs
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

	// Auth store
	store, err := auth.NewStore(machinePassphrase())
	if err != nil {
		store = nil
	}

	// Body cache
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
		bodyInput:     ta,
		responseView:  rv,
		specPathInput: si,
		baseURLInput:  bu,
		authCtxList:   al,
		newAuthInputs: newAuthInputs,
		bodyCacheList: bc,
		authStore:     store,
		bodyCache:     bodyCache,
		cfg:       cfg,
		statusMsg: "l load spec · a auth · b bodies · tab switch pane · cmd+r send",
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
		// Apply any persisted base URL override before storing the spec.
		if a.cfg != nil {
			if override, ok := a.cfg.GetBaseURLOverride(msg.source); ok {
				msg.spec.BaseURL = override
			}
		}
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

	case pagerClosedMsg:
		a.statusMsg = "Returned from pager"
		a.statusIsOk = true
	}

	switch a.mode {
	case modeLoadSpec:
		a, cmds = a.updateLoadSpec(msg, cmds)
	case modeAuthSwitch:
		a, cmds = a.updateAuthSwitch(msg, cmds)
	case modeNewAuth:
		a, cmds = a.updateNewAuth(msg, cmds)
	case modeBodyCache:
		a, cmds = a.updateBodyCache(msg, cmds)
	case modeBaseURLOverride:
		a, cmds = a.updateBaseURLOverride(msg, cmds)
	default:
		a, cmds = a.updateNormal(msg, cmds)
	}

	return a, tea.Batch(cmds...)
}

func (a App) updateNormal(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if a.inputFocused {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+c":
				return a, append(cmds, tea.Quit)
			case "esc":
				a.inputFocused = false
				a.urlInput.Blur()
				a = a.blurAllParamRows()
				a.bodyInput.Blur()
				a.statusMsg = "Command mode · tab to switch pane · ctrl+r to send"
				return a, cmds
			case "tab":
				if a.activePane == paneBody {
					// let textarea handle tab naturally
				} else if a.activeInput == 0 {
					// URL → params table
					a.activeInput = 1
					a.urlInput.Blur()
					if len(a.paramRows) > 0 {
						a = a.syncParamRowFocus()
					}
					return a, cmds
				} else {
					// Advance within params table
					a = a.advanceParamFocus()
					return a, cmds
				}
			case "shift+tab":
				if a.activePane != paneBody {
					if a.activeInput == 1 {
						a = a.retreatParamFocus()
						// If retreated past the first row, go back to URL
						// (retreatParamFocus handles staying in table)
					} else {
						// URL and shift+tab — stay on URL
					}
					return a, cmds
				}
			case "up":
				if a.activeInput == 1 {
					a = a.moveParamRowUp()
					return a, cmds
				}
			case "down":
				if a.activeInput == 1 {
					a = a.moveParamRowDown()
					return a, cmds
				}
			}
		}
		// Forward to active input
		var cmd tea.Cmd
		if a.activePane == paneBody {
			a.bodyInput, cmd = a.bodyInput.Update(msg)
		} else if a.activeInput == 0 {
			a.urlInput, cmd = a.urlInput.Update(msg)
		} else {
			a, cmd = a.updateParamsTableInput(msg)
		}
		return a, append(cmds, cmd)
	}

	// Command mode
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return a, append(cmds, tea.Quit)

		case "tab":
			a.activePane = (a.activePane + 1) % 4

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

		case "ctrl+r", "super+r":
			if a.activePane == paneRequest || a.activePane == paneBody {
				return a, append(cmds, cmdSendRequest(a))
			}

		case "y":
			if a.responseBody != "" {
				return a, append(cmds, cmdCopyToClipboard(a.responseBody))
			}

		case "o":
			if a.responseBody != "" {
				return a, append(cmds, cmdOpenInPager(a.responseBody))
			}

		case "d":
			// Delete focused param row in command mode
			if a.activePane == paneRequest && len(a.paramRows) > 0 {
				a.paramRows = append(a.paramRows[:a.paramFocused], a.paramRows[a.paramFocused+1:]...)
				if a.paramFocused >= len(a.paramRows) && a.paramFocused > 0 {
					a.paramFocused--
				}
			}

		case "enter":
			if a.activePane == paneEndpoints {
				if nav, ok := a.endpointList.SelectedItem().(NavItem); ok {
					switch nav.kind {
					case NavKindSpec, NavKindFolder:
						a.allNavItems = toggleNavItem(a.allNavItems, nav)
						a.endpointList.SetItems(visibleNavItems(a.allNavItems))
					case NavKindEndpoint:
						a.urlInput.SetValue(nav.ep.URL)
						a.paramRows = specParamRows(nav.ep.Params)
						a.paramFocused = 0
						a.paramSubFocus = 1
						a.activePane = paneRequest
						a.activeInput = 0
						a.inputFocused = true
						a.urlInput.Focus()
						a = a.blurAllParamRows()
						a.bodyInput.Blur()
						a.statusMsg = "Typing mode · esc to return to commands"
					}
				}
			} else if a.activePane == paneRequest {
				a.inputFocused = true
				a.activeInput = 0
				a.urlInput.Focus()
				a = a.blurAllParamRows()
				a.bodyInput.Blur()
				a.statusMsg = "Typing mode · esc to return to commands"
			} else if a.activePane == paneBody {
				a.inputFocused = true
				var cmd tea.Cmd
				cmd = a.bodyInput.Focus()
				a.urlInput.Blur()
				a = a.blurAllParamRows()
				a.statusMsg = "Body mode · esc to return to commands"
				return a, append(cmds, cmd)
			}

		case "p":
			// Jump to params table in request pane
			if a.activePane == paneRequest {
				a.inputFocused = true
				a.activeInput = 1
				a.urlInput.Blur()
				a.bodyInput.Blur()
				a = a.syncParamRowFocus()
				a.statusMsg = "Params mode · ↑↓ navigate · tab advance · d delete · esc command mode"
			}

		case "i":
			// Jump to body pane
			a.activePane = paneBody
			a.inputFocused = true
			var cmd tea.Cmd
			cmd = a.bodyInput.Focus()
			a.urlInput.Blur()
			a = a.blurAllParamRows()
			a.statusMsg = "Body mode · esc to return to commands"
			return a, append(cmds, cmd)

		case "u":
			if a.activePane == paneEndpoints {
				if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindSpec {
					for i, s := range a.loadedSpecs {
						if s.Source == nav.specSource {
							a.overrideTargetIdx = i
							break
						}
					}
					a.baseURLInput.SetValue(a.loadedSpecs[a.overrideTargetIdx].BaseURL)
					a.baseURLInput.Focus()
					a.mode = modeBaseURLOverride
					return a, cmds
				}
			}

		case "x":
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

	// Delegate navigation to focused pane (command mode only).
	switch a.activePane {
	case paneEndpoints:
		var cmd tea.Cmd
		a.endpointList, cmd = a.endpointList.Update(msg)
		cmds = append(cmds, cmd)
	case paneResponse:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "up", "k":
				a.responseView.Up()
			case "down", "j":
				a.responseView.Down()
			case "left", "h":
				a.responseView.Left()
			case "right", "l":
				a.responseView.Right()
			case "ctrl+f", "pgdown":
				a.responseView.PageDown()
			case "ctrl+b", "pgup":
				a.responseView.PageUp()
			case "g":
				a.responseView.GoToTop()
			case "G":
				a.responseView.GoToBottom()
			case "0":
				a.responseView.LineStart()
			case "$":
				a.responseView.LineEnd()
			case "v":
				a.responseView.ToggleSelect()
				if a.responseView.selecting {
					a.statusMsg = "Visual mode · move to select · y to copy · esc to cancel"
				} else {
					a.statusMsg = "Selection cleared"
				}
			case "y":
				sel := a.responseView.SelectedText()
				if sel == "" {
					sel = a.responseBody
				}
				cmds = append(cmds, cmdCopyToClipboard(sel))
				a.responseView.ClearSelect()
			case "esc":
				a.responseView.ClearSelect()
				a.statusMsg = "Selection cancelled"
			}
		}
	}

	return a, cmds
}

// updateParamsTableInput routes a message to the focused param row input.
// Returns the updated App and any tea.Cmd.
func (a App) updateParamsTableInput(msg tea.Msg) (App, tea.Cmd) {
	if len(a.paramRows) == 0 {
		return a, nil
	}
	row := a.paramRows[a.paramFocused]
	var cmd tea.Cmd
	if !row.fromSpec && a.paramSubFocus == 0 {
		row.keyInput, cmd = row.keyInput.Update(msg)
	} else {
		row.value, cmd = row.value.Update(msg)
	}
	a.paramRows[a.paramFocused] = row
	return a, cmd
}

// --- Param row helpers ---

func (a App) blurAllParamRows() App {
	for i := range a.paramRows {
		a.paramRows[i].keyInput.Blur()
		a.paramRows[i].value.Blur()
	}
	return a
}

func (a App) syncParamRowFocus() App {
	a = a.blurAllParamRows()
	if len(a.paramRows) == 0 {
		return a
	}
	if a.paramFocused >= len(a.paramRows) {
		a.paramFocused = len(a.paramRows) - 1
	}
	if !a.paramRows[a.paramFocused].fromSpec && a.paramSubFocus == 0 {
		a.paramRows[a.paramFocused].keyInput.Focus()
	} else {
		a.paramRows[a.paramFocused].value.Focus()
	}
	return a
}

func (a App) advanceParamFocus() App {
	if len(a.paramRows) == 0 {
		return a
	}
	a = a.blurAllParamRows()
	row := a.paramRows[a.paramFocused]
	// If on a custom row's key, advance to its value.
	if !row.fromSpec && a.paramSubFocus == 0 {
		a.paramSubFocus = 1
		a.paramRows[a.paramFocused].value.Focus()
		return a
	}
	// Advance to the next row.
	a.paramFocused++
	if a.paramFocused >= len(a.paramRows) {
		// Append a new empty row and advance to it.
		a.paramRows = append(a.paramRows, newCustomParamRow())
	}
	if a.paramRows[a.paramFocused].fromSpec {
		a.paramSubFocus = 1
		a.paramRows[a.paramFocused].value.Focus()
	} else {
		a.paramSubFocus = 0
		a.paramRows[a.paramFocused].keyInput.Focus()
	}
	return a
}

func (a App) retreatParamFocus() App {
	if len(a.paramRows) == 0 {
		return a
	}
	a = a.blurAllParamRows()
	// If on a custom row's value, go back to its key.
	if !a.paramRows[a.paramFocused].fromSpec && a.paramSubFocus == 1 {
		a.paramSubFocus = 0
		a.paramRows[a.paramFocused].keyInput.Focus()
		return a
	}
	if a.paramFocused > 0 {
		a.paramFocused--
		a.paramSubFocus = 1
		a.paramRows[a.paramFocused].value.Focus()
	}
	return a
}

func (a App) moveParamRowDown() App {
	if a.paramFocused >= len(a.paramRows)-1 {
		return a
	}
	a = a.blurAllParamRows()
	a.paramFocused++
	if a.paramRows[a.paramFocused].fromSpec {
		a.paramSubFocus = 1
		a.paramRows[a.paramFocused].value.Focus()
	} else {
		a.paramSubFocus = 0
		a.paramRows[a.paramFocused].keyInput.Focus()
	}
	return a
}

func (a App) moveParamRowUp() App {
	if a.paramFocused <= 0 {
		return a
	}
	a = a.blurAllParamRows()
	a.paramFocused--
	if a.paramRows[a.paramFocused].fromSpec {
		a.paramSubFocus = 1
		a.paramRows[a.paramFocused].value.Focus()
	} else {
		a.paramSubFocus = 0
		a.paramRows[a.paramFocused].keyInput.Focus()
	}
	return a
}

func newCustomParamRow() paramRow {
	ki := textinput.New()
	ki.Placeholder = "key"
	ki.Width = paramKeyColW
	vi := textinput.New()
	vi.Placeholder = "value"
	return paramRow{
		paramIn:  "query",
		keyInput: ki,
		value:    vi,
		fromSpec: false,
	}
}

const paramKeyColW = 18

func specParamRows(params []spec.Param) []paramRow {
	rows := make([]paramRow, len(params))
	for i, p := range params {
		vi := textinput.New()
		vi.Placeholder = "value"
		rows[i] = paramRow{
			key:      p.Name,
			paramIn:  p.In,
			value:    vi,
			fromSpec: true,
		}
	}
	return rows
}

// --- Overlay update handlers ---

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

// --- View ---

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
	case modeBaseURLOverride:
		return a.overlayView(base, "Override Base URL", a.baseURLOverrideOverlay())
	}
	return base
}

func (a App) baseView() string {
	leftW := a.width / 3
	rightW := a.width - leftW - 2

	bodyH := a.height - 2

	leftPane := a.paneStyle(paneEndpoints).Width(leftW).Height(bodyH - 2).
		Render(a.endpointList.View())

	// Split right column into 3: request/params, body, response.
	reqH := bodyH / 3
	bodyPaneH := bodyH / 3
	resH := bodyH - reqH - bodyPaneH

	reqPane := a.paneStyle(paneRequest).Width(rightW).Height(reqH - 2).
		Render(a.requestView())
	bodyPane := a.paneStyle(paneBody).Width(rightW).Height(bodyPaneH - 2).
		Render(a.bodyPaneView())
	resPane := a.paneStyle(paneResponse).Width(rightW).Height(resH - 2).
		Render(a.responseView.View())

	right := lipgloss.JoinVertical(lipgloss.Left, reqPane, bodyPane, resPane)
	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, right)

	authLabel := dimStyle.Render("no auth")
	if a.activeAuth != nil {
		authLabel = statusOK.Render("auth: " + a.activeAuth.Name)
	}

	statusStyled := dimStyle.Render(a.statusMsg)
	if a.statusIsOk {
		statusStyled = statusOK.Render(a.statusMsg)
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
	// Build a colored method badge if an endpoint is selected.
	methodBadge := ""
	if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindEndpoint {
		color, ok := methodColors[nav.ep.Method]
		if !ok {
			color = "255"
		}
		methodBadge = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).
			Render(fmt.Sprintf("%-7s", nav.ep.Method))
	}

	arrow := ""
	if a.activeInput == 0 && a.activePane == paneRequest && a.inputFocused {
		arrow = statusOK.Render("▶ ")
	}
	urlLabel := arrow + methodBadge + labelStyle.Render("URL")

	paramsLabel := labelStyle.Render("Params")
	if a.activeInput == 1 && a.activePane == paneRequest && a.inputFocused {
		paramsLabel = statusOK.Render("▶ Params")
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		urlLabel,
		a.urlInput.View(),
		"",
		paramsLabel,
		a.paramsTableView(),
		"",
		dimStyle.Render("enter URL · p params · i body · tab advance · cmd+r send · esc command"),
	)
}

func (a App) paramsTableView() string {
	if len(a.paramRows) == 0 {
		return dimStyle.Render("no params · tab to add")
	}

	const sep = " │ "
	header := fmt.Sprintf("  %-*s%sVALUE", paramKeyColW, "KEY", sep)
	lines := []string{dimStyle.Render(header)}

	for i, row := range a.paramRows {
		focused := i == a.paramFocused && a.activeInput == 1 && a.inputFocused
		prefix := "  "
		if focused {
			prefix = statusOK.Render("▶") + " "
		}

		var keyPart string
		if row.fromSpec {
			key := row.key
			if len(key) > paramKeyColW {
				key = key[:paramKeyColW]
			}
			style := dimStyle
			if focused {
				style = labelStyle
			}
			keyPart = style.Render(fmt.Sprintf("%-*s", paramKeyColW, key))
		} else {
			keyPart = row.keyInput.View()
		}

		lines = append(lines, prefix+keyPart+sep+row.value.View())
	}

	return strings.Join(lines, "\n")
}

func (a App) bodyPaneView() string {
	label := labelStyle.Render("Body")
	if a.activePane == paneBody && a.inputFocused {
		label = statusOK.Render("▶ Body")
	}
	hint := dimStyle.Render("enter to edit · s save · b load · esc command")
	if a.inputFocused && a.activePane == paneBody {
		hint = dimStyle.Render("esc command · s save · cmd+r send")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		label,
		a.bodyInput.View(),
		hint,
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

func (a App) overlayView(base, title, content string) string {
	box := overlayStyle.Width(a.width / 2).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render(title),
			"",
			content,
		),
	)
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

	reqH := bodyH / 3
	bodyPaneH := bodyH / 3
	resH := bodyH - reqH - bodyPaneH

	a.endpointList.SetSize(leftW, bodyH-4)
	a.bodyInput.SetWidth(rightW - 4)
	a.bodyInput.SetHeight(bodyPaneH - 5) // inner height minus label and hint rows
	a.responseView.SetSize(rightW-2, resH-4)
	a.authCtxList.SetSize(a.width/2-6, a.height/2)
	a.bodyCacheList.SetSize(a.width/2-6, a.height/2)

	// Update widths on param row inputs.
	valW := rightW - paramKeyColW - 6
	if valW < 10 {
		valW = 10
	}
	for i := range a.paramRows {
		a.paramRows[i].value.Width = valW
		a.paramRows[i].keyInput.Width = paramKeyColW
	}

	return a
}

func (a App) selectedOperationID() string {
	if nav, ok := a.endpointList.SelectedItem().(NavItem); ok && nav.kind == NavKindEndpoint {
		return nav.ep.EndpointKey()
	}
	return ""
}

func (a App) updateBaseURLOverride(msg tea.Msg, cmds []tea.Cmd) (App, []tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			a.mode = modeNormal
			return a, cmds
		case "ctrl+d":
			// Clear override — restore the spec's original base URL.
			if a.overrideTargetIdx < len(a.loadedSpecs) {
				s := a.loadedSpecs[a.overrideTargetIdx]
				a.loadedSpecs[a.overrideTargetIdx].BaseURL = s.OriginalBaseURL
				if a.cfg != nil {
					a.cfg.ClearBaseURLOverride(s.Source)
					_ = a.cfg.Save()
				}
				a.allNavItems = buildNavItems(a.loadedSpecs)
				a.endpointList.SetItems(visibleNavItems(a.allNavItems))
				a.statusMsg = "Base URL override cleared"
				a.statusIsOk = true
			}
			a.mode = modeNormal
			return a, cmds
		case "enter":
			newURL := strings.TrimRight(strings.TrimSpace(a.baseURLInput.Value()), "/")
			if a.overrideTargetIdx < len(a.loadedSpecs) {
				a.loadedSpecs[a.overrideTargetIdx].BaseURL = newURL
				if a.cfg != nil {
					a.cfg.SetBaseURLOverride(a.loadedSpecs[a.overrideTargetIdx].Source, newURL)
					_ = a.cfg.Save()
				}
				a.allNavItems = buildNavItems(a.loadedSpecs)
				a.endpointList.SetItems(visibleNavItems(a.allNavItems))
				a.statusMsg = "Base URL updated: " + newURL
				a.statusIsOk = true
			}
			a.mode = modeNormal
			return a, cmds
		}
	}
	var cmd tea.Cmd
	a.baseURLInput, cmd = a.baseURLInput.Update(msg)
	cmds = append(cmds, cmd)
	return a, cmds
}

func (a App) baseURLOverrideOverlay() string {
	spec := a.loadedSpecs[a.overrideTargetIdx]
	lines := []string{
		labelStyle.Render(spec.Title),
		dimStyle.Render("Original: " + spec.OriginalBaseURL),
		"",
		a.baseURLInput.View(),
		"",
		dimStyle.Render("enter to save · ctrl+d clear override · esc cancel"),
	}
	return strings.Join(lines, "\n")
}

// machinePassphrase derives a stable passphrase from the hostname.
func machinePassphrase() string {
	h, err := os.Hostname()
	if err != nil {
		h = "curlx-default"
	}
	return "curlx-" + h
}
