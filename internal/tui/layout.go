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
	b.buf.WriteString("┌")
	b.buf.WriteString(strings.Repeat("─", w-2))
	b.buf.WriteString("┐")

	// Side borders
	for i := 1; i < h-1; i++ {
		b.buf.WriteString(MoveCursor(x, y+i))
		b.buf.WriteString("│")
		b.buf.WriteString(MoveCursor(x+w-1, y+i))
		b.buf.WriteString("│")
	}

	// Bottom border
	b.buf.WriteString(MoveCursor(x, y+h-1))
	b.buf.WriteString("└")
	b.buf.WriteString(strings.Repeat("─", w-2))
	b.buf.WriteString("┘")
	b.buf.WriteString(Reset)
}

// Render flushes the buffer to a string so it can be written to os.Stdout atomically.
func (b *Buffer) Render() string {
	return b.buf.String()
}
// DrawLine draws a straight line between two points using Bresenham's algorithm.
func (b *Buffer) DrawLine(x0, y0, x1, y1 int, ch string, color string) {
	dx := x1 - x0
	if dx < 0 { dx = -dx }
	dy := y1 - y0
	if dy < 0 { dy = -dy }
	
	sx := 1
	if x0 > x1 { sx = -1 }
	sy := 1
	if y0 > y1 { sy = -1 }
	
	err := dx - dy
	
	for {
		b.PrintAt(x0, y0, ch, color)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// WordWrap splits a string into multiple lines with a maximum width.
func WordWrap(text string, maxWidth int) []string {
	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return lines
	}
	
	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)
	return lines
}
