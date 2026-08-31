package tui

import (
	"bytes"
	"strings"
)

// Buffer represents a double-buffered off-screen canvas.
// Instead of printing directly to os.Stdout (which causes flicker),
// we draw characters into a bytes.Buffer and flush them atomically.
type Buffer struct {
	buf bytes.Buffer
}

// NewBuffer creates a new render buffer.
func NewBuffer() *Buffer {
	return &Buffer{}
}

// Clear resets the buffer and the terminal screen.
func (b *Buffer) Clear() {
	b.buf.Reset()
	b.buf.WriteString(ClearScreen)
}

// WriteString adds a string to the buffer at the current cursor position.
func (b *Buffer) WriteString(s string) {
	b.buf.WriteString(s)
}

// PrintAt moves the cursor to (x, y) and prints the colored text.
func (b *Buffer) PrintAt(x, y int, text, color string) {
	b.buf.WriteString(MoveCursor(x, y))
	b.buf.WriteString(color)
	b.buf.WriteString(text)
	b.buf.WriteString(Reset)
}

// DrawBox draws an ASCII border box at (x,y) with width w and height h.
func (b *Buffer) DrawBox(x, y, w, h int, color string) {
	b.buf.WriteString(color)
	
	// Top border
	b.buf.WriteString(MoveCursor(x, y))
	b.buf.WriteString("┌" + strings.Repeat("─", w-2) + "┐")

	// Side borders
	for i := 1; i < h-1; i++ {
		b.buf.WriteString(MoveCursor(x, y+i))
		b.buf.WriteString("│")
		b.buf.WriteString(MoveCursor(x+w-1, y+i))
		b.buf.WriteString("│")
	}

	// Bottom border
	b.buf.WriteString(MoveCursor(x, y+h-1))
	b.buf.WriteString("└" + strings.Repeat("─", w-2) + "┘")
	b.buf.WriteString(Reset)
}

// Render flushes the buffer to a string so it can be written to os.Stdout atomically.
func (b *Buffer) Render() string {
	return b.buf.String()
}
