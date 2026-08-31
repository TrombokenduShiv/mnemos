package tui

import (
	"fmt"
)

// Terminal Control Sequences
const (
	ClearScreen = "\033[2J\033[H"
	HideCursor  = "\033[?25l"
	ShowCursor  = "\033[?25h"
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Italic      = "\033[3m"
)

// RGB returns a 24-bit True Color ANSI foreground escape sequence.
func RGB(r, g, b int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// BgRGB returns a 24-bit True Color ANSI background escape sequence.
func BgRGB(r, g, b int) string {
	return fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)
}

// MoveCursor returns the ANSI sequence to move the cursor to (x, y) where 1,1 is top-left.
func MoveCursor(x, y int) string {
	return fmt.Sprintf("\033[%d;%dH", y, x)
}

// Cyberpunk / Retro Theme Colors
var (
	BgVoid      = BgRGB(10, 10, 16)
	FgInkHi     = RGB(240, 244, 248)
	FgInkMd     = RGB(148, 163, 184)
	FgInkLo     = RGB(100, 116, 139)
	FgAccent    = RGB(94, 234, 212) // Neon Cyan
	FgAccentDim = RGB(45, 112, 101)
	FgViolet    = RGB(167, 139, 250)
	FgPink      = RGB(251, 113, 133) // Hot Pink
	FgEmerald   = RGB(52, 211, 153)
	FgAmber     = RGB(251, 191, 36)
	FgBorder    = RGB(60, 60, 80)
)
