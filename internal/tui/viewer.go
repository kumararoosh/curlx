package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	cursorStyle    = lipgloss.NewStyle().Reverse(true)
	selectionStyle = lipgloss.NewStyle().Background(lipgloss.Color("25")).Foreground(lipgloss.Color("255"))
)

// viewer is a read-only text viewer with keyboard cursor and visual selection.
type viewer struct {
	lines     []string
	curLine   int
	curCol    int
	selecting bool
	selLine   int
	selCol    int
	topLine   int // vertical scroll offset
	width     int
	height    int
}

func newViewer(width, height int) viewer {
	return viewer{width: width, height: height}
}

func (v *viewer) SetContent(s string) {
	v.lines = strings.Split(s, "\n")
	v.curLine = 0
	v.curCol = 0
	v.topLine = 0
	v.selecting = false
}

func (v *viewer) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.clampScroll()
}

// --- Cursor movement ---

func (v *viewer) Up() {
	if v.curLine > 0 {
		v.curLine--
		v.clampCol()
		v.clampScroll()
	}
}

func (v *viewer) Down() {
	if v.curLine < len(v.lines)-1 {
		v.curLine++
		v.clampCol()
		v.clampScroll()
	}
}

func (v *viewer) Left() {
	if v.curCol > 0 {
		v.curCol--
	} else if v.curLine > 0 {
		v.curLine--
		v.curCol = len(v.lines[v.curLine])
		v.clampScroll()
	}
}

func (v *viewer) Right() {
	lineLen := len(v.lines[v.curLine])
	if v.curCol < lineLen {
		v.curCol++
	} else if v.curLine < len(v.lines)-1 {
		v.curLine++
		v.curCol = 0
		v.clampScroll()
	}
}

func (v *viewer) PageDown() {
	v.curLine = min(v.curLine+v.height, len(v.lines)-1)
	v.clampCol()
	v.clampScroll()
}

func (v *viewer) PageUp() {
	v.curLine = max(v.curLine-v.height, 0)
	v.clampCol()
	v.clampScroll()
}

func (v *viewer) GoToTop() {
	v.curLine = 0
	v.curCol = 0
	v.topLine = 0
}

func (v *viewer) GoToBottom() {
	v.curLine = len(v.lines) - 1
	v.curCol = 0
	v.clampScroll()
}

func (v *viewer) LineStart() { v.curCol = 0 }

func (v *viewer) LineEnd() {
	v.curCol = len(v.lines[v.curLine])
}

// --- Selection ---

// ToggleSelect starts selection at the current cursor, or clears it if active.
func (v *viewer) ToggleSelect() {
	if v.selecting {
		v.selecting = false
	} else {
		v.selecting = true
		v.selLine = v.curLine
		v.selCol = v.curCol
	}
}

func (v *viewer) ClearSelect() {
	v.selecting = false
}

// SelectedText returns the text in the current selection, or "" if none.
func (v viewer) SelectedText() string {
	if !v.selecting {
		return ""
	}
	sl, sc, el, ec := v.normalizedSelection()

	if sl == el {
		line := v.lines[sl]
		end := min(ec+1, len(line))
		if sc >= len(line) {
			return ""
		}
		return line[sc:end]
	}

	var parts []string
	if sc < len(v.lines[sl]) {
		parts = append(parts, v.lines[sl][sc:])
	}
	for l := sl + 1; l < el; l++ {
		parts = append(parts, v.lines[l])
	}
	if el < len(v.lines) {
		line := v.lines[el]
		end := min(ec+1, len(line))
		if end > 0 {
			parts = append(parts, line[:end])
		}
	}
	return strings.Join(parts, "\n")
}

// --- Rendering ---

func (v viewer) View() string {
	if len(v.lines) == 0 {
		return ""
	}

	var sb strings.Builder
	end := min(v.topLine+v.height, len(v.lines))

	for i, lineIdx := 0, v.topLine; lineIdx < end; i, lineIdx = i+1, lineIdx+1 {
		line := v.lines[lineIdx]

		// Iterate rune-by-rune so we can apply per-character styles.
		col := 0
		for _, ch := range line {
			if col >= v.width {
				break
			}
			s := string(ch)
			switch {
			case v.inSelection(lineIdx, col):
				sb.WriteString(selectionStyle.Render(s))
			case lineIdx == v.curLine && col == v.curCol:
				sb.WriteString(cursorStyle.Render(s))
			default:
				sb.WriteString(s)
			}
			col++
		}

		// Cursor sitting past end of line (empty line or end position).
		if lineIdx == v.curLine && v.curCol >= col {
			sb.WriteString(cursorStyle.Render(" "))
		}

		if lineIdx < end-1 {
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}

// --- Helpers ---

func (v viewer) inSelection(line, col int) bool {
	if !v.selecting {
		return false
	}
	sl, sc, el, ec := v.normalizedSelection()
	if line < sl || line > el {
		return false
	}
	if line == sl && col < sc {
		return false
	}
	if line == el && col > ec {
		return false
	}
	return true
}

func (v viewer) normalizedSelection() (sl, sc, el, ec int) {
	if v.selLine < v.curLine || (v.selLine == v.curLine && v.selCol <= v.curCol) {
		return v.selLine, v.selCol, v.curLine, v.curCol
	}
	return v.curLine, v.curCol, v.selLine, v.selCol
}

func (v *viewer) clampCol() {
	lineLen := len(v.lines[v.curLine])
	if v.curCol > lineLen {
		v.curCol = lineLen
	}
}

func (v *viewer) clampScroll() {
	if v.curLine < v.topLine {
		v.topLine = v.curLine
	}
	if v.curLine >= v.topLine+v.height {
		v.topLine = v.curLine - v.height + 1
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
