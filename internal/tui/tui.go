package tui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"mnemos/internal/server"
)

// Mnemos interface matches the storage engine's Search and GetStats methods.
type Mnemos interface {
	Search(query string, k int) ([]server.SearchResult, [2]float64, []server.DataPoint, error)
	GetStats() map[string]interface{}
}

type appState struct {
	mu           sync.Mutex
	query        string
	loading      bool
	results      []server.SearchResult
	queryPoint   [2]float64
	allPoints    []server.DataPoint
	latency      string
	stats        map[string]interface{}
	frameCounter int
}

// Run starts the interactive TUI.
func Run(engine Mnemos) {
	// 1. Enter raw mode to capture individual keystrokes.
	cleanup, err := RawMode()
	if err != nil {
		fmt.Printf("Failed to enter raw mode: %v\n", err)
		return
	}
	defer cleanup()

	// Hide cursor on startup, restore on exit
	fmt.Print(HideCursor)
	defer fmt.Print(ShowCursor)
	defer fmt.Print(Reset)
	defer fmt.Print(ClearScreen)

	state := &appState{
		stats: engine.GetStats(),
	}

	// 2. Start a background goroutine for telemetry polling
	go func() {
		for {
			time.Sleep(1 * time.Second)
			st := engine.GetStats()
			state.mu.Lock()
			state.stats = st
			state.mu.Unlock()
		}
	}()

	// 3. Start a background goroutine for reading keyboard input
	inputCh := make(chan []byte)
	go func() {
		b := make([]byte, 128)
		for {
			n, err := os.Stdin.Read(b)
			if err != nil || n == 0 {
				continue
			}
			data := make([]byte, n)
			copy(data, b[:n])
			inputCh <- data
		}
	}()

	// 4. Main Render Loop (30 FPS)
	ticker := time.NewTicker(time.Second / 30)
	defer ticker.Stop()

	for {
		select {
		case in := <-inputCh:
			// Process input
			if len(in) == 1 && (in[0] == 3 || in[0] == 27) {
				// Ctrl+C (3) or Escape (27) to quit
				return
			}
			
			state.mu.Lock()
			if len(in) == 1 && in[0] == 127 || in[0] == 8 {
				// Backspace
				if len(state.query) > 0 {
					state.query = state.query[:len(state.query)-1]
				}
			} else if len(in) == 1 && in[0] == 13 {
				// Enter key -> Trigger Search
				if len(state.query) > 0 {
					state.loading = true
					state.results = nil
					q := state.query
					
					// Run search asynchronously
					go func(searchQuery string) {
						start := time.Now()
						res, qp, pts, _ := engine.Search(searchQuery, 3)
						lat := fmt.Sprintf("%dms", time.Since(start).Milliseconds())
						
						state.mu.Lock()
						state.results = res
						state.queryPoint = qp
						state.allPoints = pts
						state.latency = lat
						state.loading = false
						state.mu.Unlock()
					}(q)
				}
			} else {
				// Append printable characters
				str := string(in)
				// Filter out ANSI escape sequences (like arrow keys) for simplicity
				if !strings.HasPrefix(str, "\033") {
					state.query += str
				}
			}
			state.mu.Unlock()

		case <-ticker.C:
			// Render Frame
			state.mu.Lock()
			state.frameCounter++
			
			buf := NewBuffer()
			buf.Clear()
			
			// Fill background void
			buf.WriteString(BgVoid)
			
			drawHeader(buf)
			drawSearchBar(buf, state)
			drawResults(buf, state)
			drawTelemetry(buf, state)
			drawVisualizer(buf, state)

			// Atomic flush
			fmt.Print(buf.Render())
			
			state.mu.Unlock()
		}
	}
}

func drawHeader(buf *Buffer) {
	// Retro ASCII Logo
	logo := []string{
		`███╗   ███╗███╗   ██╗███████╗███╗   ███╗ ██████╗  ██████╗ `,
		`████╗ ████║████╗  ██║██╔════╝████╗ ████║██╔═══██╗██╔════╝ `,
		`██╔████╔██║██╔██╗ ██║█████╗  ██╔████╔██║██║   ██║╚█████╗  `,
		`██║╚██╔╝██║██║╚██╗██║██╔══╝  ██║╚██╔╝██║██║   ██║ ╚═══██╗ `,
		`██║ ╚═╝ ██║██║ ╚████║███████╗██║ ╚═╝ ██║╚██████╔╝██████╔╝ `,
		`╚═╝     ╚═╝╚═╝  ╚═══╝╚══════╝╚═╝     ╚═╝ ╚═════╝ ╚═════╝  `,
	}

	startX, startY := 4, 2
	for i, line := range logo {
		// Gradient logic: Blue to Pink
		r := 94 + (i * 20)
		g := 234 - (i * 20)
		b := 212 + (i * 5)
		buf.PrintAt(startX, startY+i, line, RGB(r, g, b))
	}

	buf.PrintAt(startX+2, startY+6, "ZERO-DEPENDENCY SEMANTIC MEMORY ENGINE", FgInkLo)
}

func drawSearchBar(buf *Buffer, state *appState) {
	buf.DrawBox(4, 10, 60, 3, FgBorder)
	
	// Prompt indicator
	buf.PrintAt(6, 11, ">", FgAccent)
	
	// Query text
	buf.PrintAt(8, 11, state.query, FgInkHi)
	
	// Blinking cursor
	if state.frameCounter%30 < 15 {
		buf.PrintAt(8+len(state.query), 11, "█", FgAccent)
	}
}

func drawResults(buf *Buffer, state *appState) {
	startY := 14
	if state.loading {
		frames := []string{"[-]", "[\\]", "[|]", "[/]"}
		anim := frames[(state.frameCounter/4)%4]
		buf.PrintAt(4, startY, fmt.Sprintf("%s Extracting semantic features...", anim), FgViolet)
		return
	}

	if state.results == nil {
		buf.PrintAt(4, startY, "Ready to Search", FgInkHi+Bold)
		buf.PrintAt(4, startY+1, "Type a natural-language question. The engine will retrieve documents...", FgInkLo)
		return
	}

	if len(state.results) == 0 {
		buf.PrintAt(4, startY, "No results found for query.", FgPink)
		return
	}

	buf.PrintAt(4, startY, fmt.Sprintf("Found %d results in %s", len(state.results), state.latency), FgAccent)
	
	y := startY + 2
	for i, res := range state.results {
		if i >= 3 { break } // Max 3 results to fit terminal
		
		// Rank and Title
		buf.PrintAt(4, y, fmt.Sprintf("#%d %s", i+1, res.Title), FgInkHi+Bold)
		// Path
		buf.PrintAt(4, y+1, res.Path, FgInkLo)
		// Scores
		scores := fmt.Sprintf("BM25: %.2f  Embed: %.2f  Fused: %.2f", res.BM25Score, res.EmbedScore, res.Score)
		buf.PrintAt(4, y+2, scores, FgViolet)
		
		// Snippet (truncate to 50 chars for TUI)
		snip := res.Snippet
		if len(snip) > 55 {
			snip = snip[:52] + "..."
		}
		buf.PrintAt(4, y+3, snip, FgInkMd)
		
		y += 5
	}
}

func drawVisualizer(buf *Buffer, state *appState) {
	vx, vy := 66, 2
	vw, vh := 40, 16
	
	buf.DrawBox(vx, vy, vw, vh, FgBorder)
	buf.PrintAt(vx+2, vy, " VECTOR SPACE ", FgInkHi+Bold)

	if len(state.allPoints) == 0 {
		buf.PrintAt(vx+(vw/2)-4, vy+(vh/2), "AWAITING", FgInkLo)
		return
	}

	// Plot dots
	for _, p := range state.allPoints {
		// Map [-1, 1] to [1, vw-2]
		cx := int(((p.X + 1.0) / 2.0) * float64(vw-2))
		cy := int(((p.Y + 1.0) / 2.0) * float64(vh-2))
		
		if cx > 0 && cx < vw-1 && cy > 0 && cy < vh-1 {
			buf.PrintAt(vx+cx, vy+cy, ".", FgInkLo)
		}
	}

	// Plot results in Pink
	for _, res := range state.results {
		for _, p := range state.allPoints {
			if p.ID == res.DocID {
				cx := int(((p.X + 1.0) / 2.0) * float64(vw-2))
				cy := int(((p.Y + 1.0) / 2.0) * float64(vh-2))
				if cx > 0 && cx < vw-1 && cy > 0 && cy < vh-1 {
					buf.PrintAt(vx+cx, vy+cy, "●", FgPink)
				}
				break
			}
		}
	}

	// Plot query point in Cyan
	if !state.loading && state.results != nil {
		cx := int(((state.queryPoint[0] + 1.0) / 2.0) * float64(vw-2))
		cy := int(((state.queryPoint[1] + 1.0) / 2.0) * float64(vh-2))
		if cx > 0 && cx < vw-1 && cy > 0 && cy < vh-1 {
			// Pulse effect
			char := "O"
			if state.frameCounter%10 < 5 { char = "o" }
			buf.PrintAt(vx+cx, vy+cy, char, FgAccent)
		}
	}
}

func drawTelemetry(buf *Buffer, state *appState) {
	tx, ty := 66, 19
	tw, th := 40, 9
	
	buf.DrawBox(tx, ty, tw, th, FgBorder)
	buf.PrintAt(tx+2, ty, " TELEMETRY ", FgInkHi+Bold)
	
	// Blinking LIVE indicator
	if state.frameCounter%30 < 15 {
		buf.PrintAt(tx+tw-8, ty, "● LIVE", FgEmerald)
	} else {
		buf.PrintAt(tx+tw-8, ty, "  LIVE", FgAccentDim)
	}

	y := ty + 2
	printStat := func(label string, value interface{}) {
		buf.PrintAt(tx+2, y, label, FgInkLo)
		buf.PrintAt(tx+15, y, fmt.Sprintf("%v", value), FgInkHi)
		y++
	}

	if state.stats != nil {
		printStat("DOCUMENTS :", state.stats["document_count"])
		printStat("VOCABULARY:", state.stats["vocabulary_size"])
		printStat("SSTABLES  :", state.stats["storage_sstable_count"])
		
		var mem float64
		switch v := state.stats["storage_memtable_size"].(type) {
		case float64:
			mem = v
		case int:
			mem = float64(v)
		case int64:
			mem = float64(v)
		}
		if mem > 1024 {
			printStat("MEMTABLE  :", fmt.Sprintf("%.1f KB", mem/1024))
		} else {
			printStat("MEMTABLE  :", fmt.Sprintf("%.0f B", mem))
		}
	}
	printStat("LATENCY   :", state.latency)
}
