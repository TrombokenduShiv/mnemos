// Package server implements a minimal HTTP/1.1 server with an embedded query UI
// and JSON API. No external framework — just net/http ServeMux with hand-written handlers.
// Replaces Gin, Echo, Express, Flask, and any frontend framework/CDN.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// DataPoint represents a 2D coordinate in the embedding space for visualization.
type DataPoint struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// Mnemos is the interface the server uses to query the search engine.
type Mnemos interface {
	Search(query string, k int) ([]SearchResult, [2]float64, []DataPoint, error)
	GetStats() map[string]interface{}
}

// SearchResult represents a single search result from the engine.
type SearchResult struct {
	DocID      string  `json:"doc_id"`
	Title      string  `json:"title"`
	Path       string  `json:"path"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	BM25Score  float64 `json:"bm25_score"`
	EmbedScore float64 `json:"embed_score"`
	Rank       int     `json:"rank"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}

// Server is the HTTP server for Mnemos.
type Server struct {
	engine Mnemos
	mux    *http.ServeMux
	server *http.Server
}

// New creates a new HTTP server.
func New(engine Mnemos, addr string) *Server {
	s := &Server{
		engine: engine,
		mux:    http.NewServeMux(),
	}

	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/api/query", s.handleQuery)
	s.mux.HandleFunc("/api/stats", s.handleStats)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	log.Printf("Mnemos server listening on http://%s\n", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleIndex serves the embedded HTML UI.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

// handleQuery processes search queries via JSON API.
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(200)
		return
	}

	var query string
	k := 10

	if r.Method == "POST" {
		var req struct {
			Q string `json:"q"`
			K int    `json:"k"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, 400)
			return
		}
		query = req.Q
		if req.K > 0 {
			k = req.K
		}
	} else {
		query = r.URL.Query().Get("q")
		if kStr := r.URL.Query().Get("k"); kStr != "" {
			if parsed, err := strconv.Atoi(kStr); err == nil && parsed > 0 {
				k = parsed
			}
		}
	}

	if query == "" {
		http.Error(w, `{"error":"query is required"}`, 400)
		return
	}

	start := time.Now()
	results, queryPoint, allPoints, err := s.engine.Search(query, k)
	elapsed := time.Since(start)

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 500)
		return
	}

	resp := struct {
		Query      string         `json:"query"`
		Results    []SearchResult `json:"results"`
		QueryPoint [2]float64     `json:"query_point"`
		AllPoints  []DataPoint    `json:"all_points"`
		Count      int            `json:"count"`
		LatencyMs  float64        `json:"latency_ms"`
	}{
		Query:      query,
		Results:    results,
		QueryPoint: queryPoint,
		AllPoints:  allPoints,
		Count:      len(results),
		LatencyMs:  float64(elapsed.Microseconds()) / 1000.0,
	}

	json.NewEncoder(w).Encode(resp)
}

// handleStats returns engine statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := s.engine.GetStats()
	json.NewEncoder(w).Encode(stats)
}

// indexHTML is the complete embedded HTML/CSS/JS UI — no external dependencies.
// Adheres strictly to the Interface Design Guide constraints.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mnemos</title>
<style>
  :root {
    --bg-void: #0B1016;
    --bg-panel: #141B23;
    --bg-panel-raised: #1B2530;
    --trace-cyan: #5EEAD4;
    --trace-amber: #F5A623;
    --trace-violet: #A78BFA;
    --ink-hi: #E8EDF2;
    --ink-lo: #7C8895;
    
    --font-display: system-ui, -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
    --font-body: system-ui, -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
    --font-mono: ui-monospace, SFMono-Regular, Consolas, 'Liberation Mono', Menlo, monospace;
  }

  * { box-sizing: border-box; margin: 0; padding: 0; }

  body {
    background-color: var(--bg-void);
    color: var(--ink-hi);
    font-family: var(--font-body);
    line-height: 1.5;
    min-height: 100vh;
    display: flex;
    justify-content: center;
  }

  .layout {
    display: grid;
    grid-template-columns: 1fr 340px;
    gap: 2rem;
    width: 100%;
    max-width: 1200px;
    padding: 2rem;
  }

  /* Header */
  header {
    margin-bottom: 2rem;
  }
  
  h1 {
    font-family: var(--font-display);
    font-size: 2rem;
    font-weight: 600;
    letter-spacing: -0.02em;
    margin-bottom: 0.25rem;
  }

  /* Query Bar */
  .query-box {
    display: flex;
    position: relative;
    margin-bottom: 2rem;
  }
  
  .query-input {
    width: 100%;
    background: var(--bg-panel);
    border: 1px solid rgba(124, 136, 149, 0.3);
    color: var(--ink-hi);
    font-family: var(--font-body);
    font-size: 1.1rem;
    padding: 1rem;
    outline: none;
    transition: border-color 0.2s ease;
  }

  .query-input:focus {
    border-color: var(--trace-cyan);
  }

  .query-input::placeholder {
    color: var(--ink-lo);
  }

  .search-btn {
    background: var(--bg-panel-raised);
    border: 1px solid rgba(124, 136, 149, 0.3);
    border-left: none;
    color: var(--ink-hi);
    padding: 0 1.5rem;
    cursor: pointer;
    font-family: var(--font-body);
    font-weight: 500;
    transition: background 0.2s ease, color 0.2s ease;
  }

  .search-btn:hover {
    background: var(--trace-cyan);
    color: var(--bg-void);
  }
  
  .search-btn:focus-visible {
    outline: 2px solid var(--trace-cyan);
    outline-offset: 2px;
  }

  .query-trace {
    position: absolute;
    bottom: -1px;
    left: 0;
    height: 1px;
    background: var(--trace-cyan);
    width: 0%;
    transition: width 0.3s ease;
  }
  .query-box.loading .query-trace {
    width: 100%;
  }

  /* Results */
  .results-container {
    display: flex;
    flex-direction: column;
  }

  .result-row {
    padding: 1.5rem 0;
    border-bottom: 1px solid rgba(124, 136, 149, 0.2);
    opacity: 0;
    transform: translateY(10px);
    animation: resolveIn 0.4s ease forwards;
  }
  
  @keyframes resolveIn {
    to { opacity: 1; transform: translateY(0); }
  }

  .result-header {
    margin-bottom: 0.5rem;
  }

  .result-title {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--ink-hi);
    margin-right: 0.5rem;
  }
  
  .result-path {
    font-family: var(--font-mono);
    font-size: 0.8rem;
    color: var(--ink-lo);
  }

  .result-scores {
    font-family: var(--font-mono);
    font-size: 0.85rem;
    color: var(--ink-lo);
    margin-bottom: 1rem;
  }

  .result-snippet {
    font-size: 0.95rem;
    color: var(--ink-hi);
    border-left: 2px solid rgba(94, 234, 212, 0.3);
    padding-left: 1rem;
  }

  /* Instrument Strip */
  .instrument-strip {
    background: var(--bg-panel);
    border: 1px solid rgba(124, 136, 149, 0.2);
    padding: 1.5rem;
    display: flex;
    flex-direction: column;
    gap: 2rem;
    height: fit-content;
    position: sticky;
    top: 2rem;
  }

  .panel-section-title {
    font-family: var(--font-display);
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ink-lo);
    margin-bottom: 1rem;
    display: flex;
    justify-content: space-between;
  }

  /* Vector Visualizer */
  .visualizer-container {
    width: 100%;
    aspect-ratio: 1;
    background: var(--bg-void);
    border: 1px solid rgba(124, 136, 149, 0.2);
    position: relative;
    overflow: hidden;
  }
  
  svg {
    width: 100%;
    height: 100%;
  }

  .viz-point {
    fill: var(--ink-lo);
    opacity: 0.4;
    transition: transform 10s ease-in-out;
  }
  
  .viz-bucket {
    stroke: var(--trace-violet);
    stroke-width: 1;
    stroke-dasharray: 4 4;
    opacity: 0.3;
  }

  .viz-query {
    fill: var(--trace-cyan);
    animation: pulse 2s infinite;
  }

  .viz-trace {
    stroke: var(--trace-cyan);
    stroke-width: 1.5;
    opacity: 0.6;
    stroke-dasharray: 1000;
    stroke-dashoffset: 1000;
    animation: dash 1s ease-out forwards;
  }
  
  @keyframes dash {
    to { stroke-dashoffset: 0; }
  }

  @keyframes pulse {
    0% { r: 3; opacity: 1; }
    50% { r: 6; opacity: 0.5; }
    100% { r: 3; opacity: 1; }
  }

  /* Stats / Meters */
  .stat-row {
    display: flex;
    justify-content: space-between;
    margin-bottom: 0.5rem;
    font-family: var(--font-mono);
    font-size: 0.85rem;
  }

  .stat-label { color: var(--ink-lo); }
  .stat-val { color: var(--ink-hi); }
  .stat-val.accent { color: var(--trace-cyan); }

  .sparkline {
    width: 100%;
    height: 30px;
    margin-top: 0.5rem;
    border-bottom: 1px solid rgba(124, 136, 149, 0.2);
  }
  
  .spark-path {
    fill: none;
    stroke: var(--trace-amber);
    stroke-width: 1.5;
  }

  .msg {
    color: var(--ink-lo);
    font-size: 0.95rem;
    padding: 2rem 0;
  }
  .msg-error {
    color: var(--trace-amber);
  }

  /* Drawer Toggle (Mobile) */
  .drawer-toggle {
    display: none;
    width: 100%;
    padding: 1rem;
    background: var(--bg-panel);
    border: 1px solid rgba(124, 136, 149, 0.2);
    color: var(--ink-hi);
    font-family: var(--font-display);
    text-align: center;
    cursor: pointer;
    margin-bottom: 1rem;
  }

  @media (max-width: 900px) {
    .layout {
      grid-template-columns: 1fr;
    }
    .drawer-toggle {
      display: block;
    }
    .instrument-strip {
      display: none;
      position: static;
    }
    .instrument-strip.open {
      display: flex;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .result-row { animation: none; opacity: 1; transform: none; }
    .viz-query { animation: none; }
    .viz-trace { animation: none; stroke-dashoffset: 0; }
    .viz-point { transition: none; }
  }
</style>
</head>
<body>

<div class="layout">
  <main>
    <header>
      <h1>MNEMOS</h1>
    </header>
    
    <div class="query-box" id="queryBox">
      <input type="text" class="query-input" id="queryInput" placeholder="ask a question about your documents" autofocus>
      <button class="search-btn" onclick="submitQuery()">Search</button>
      <div class="query-trace"></div>
    </div>

    <div id="results" class="results-container">
      <div class="msg">No search executed. Enter a query above.</div>
    </div>
  </main>

  <aside>
    <button class="drawer-toggle" onclick="toggleDrawer()">Toggle Instrument Strip</button>
    <div class="instrument-strip" id="instrumentStrip">
      
      <div class="section">
        <div class="panel-section-title">Vector Space Visualizer</div>
        <div class="visualizer-container" id="vizContainer">
          <!-- SVG injected here -->
        </div>
      </div>

      <div class="section">
        <div class="panel-section-title">Engine Telemetry</div>
        <div class="stat-row">
          <span class="stat-label">SSTables</span>
          <span class="stat-val" id="statSSTables">-</span>
        </div>
        <div class="stat-row">
          <span class="stat-label">Memtable Size</span>
          <span class="stat-val" id="statMemtable">-</span>
        </div>
        <div class="stat-row">
          <span class="stat-label">Doc Count</span>
          <span class="stat-val" id="statDocs">-</span>
        </div>
        <div class="stat-row">
          <span class="stat-label">Vocab Size</span>
          <span class="stat-val" id="statVocab">-</span>
        </div>
        <div class="stat-row" style="margin-top: 1rem">
          <span class="stat-label">Query Latency</span>
          <span class="stat-val accent" id="statLatency">- ms</span>
        </div>
      </div>

      <div class="section">
        <div class="panel-section-title">
          <span>WAL Activity</span>
          <span style="color: var(--trace-amber)">Live</span>
        </div>
        <svg class="sparkline" id="walSparkline" viewBox="0 0 100 30" preserveAspectRatio="none">
          <path class="spark-path" d="M0,30 L100,30"></path>
        </svg>
      </div>

    </div>
  </aside>
</div>

<script>
// --- UI Logic ---
function toggleDrawer() {
  document.getElementById('instrumentStrip').classList.toggle('open');
}

const queryInput = document.getElementById('queryInput');
queryInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') submitQuery();
});

// --- API Calls ---
async function submitQuery() {
  const q = queryInput.value.trim();
  if (!q) return;

  document.getElementById('queryBox').classList.add('loading');
  const resultsDiv = document.getElementById('results');
  resultsDiv.innerHTML = '<div class="msg">Reading...</div>';

  try {
    const res = await fetch('/api/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ q, k: 10 })
    });
    
    const data = await res.json();
    document.getElementById('queryBox').classList.remove('loading');

    if (data.error) {
      resultsDiv.innerHTML = '<div class="msg msg-error">' + escapeHtml(data.error) + '</div>';
      return;
    }

    if (!data.results || data.results.length === 0) {
      resultsDiv.innerHTML = '<div class="msg">No matches for that question. Try different words, or check that the right documents are ingested.</div>';
      return;
    }

    // Update Latency
    document.getElementById('statLatency').textContent = data.latency_ms.toFixed(1) + ' ms';

    // Render Results
    let html = '';
    data.results.forEach((r, i) => {
      // Small stagger delay for rendering
      const delay = i * 0.05;
      html += '<div class="result-row" style="animation-delay: ' + delay + 's">';
      html += '<div class="result-header"><span class="result-title">' + escapeHtml(r.title) + '</span> <span class="result-path">' + escapeHtml(r.path) + '</span></div>';
      html += '<div class="result-scores">bm25 ' + r.bm25_score.toFixed(2) + ' · embedding ' + r.embed_score.toFixed(2) + ' · fused ' + r.score.toFixed(2) + '</div>';
      html += '<div class="result-snippet">' + escapeHtml(r.snippet) + '</div>';
      html += '</div>';
    });
    resultsDiv.innerHTML = html;

    // Render Visualizer
    renderVisualizer(data.all_points, data.query_point, data.results);

  } catch (err) {
    document.getElementById('queryBox').classList.remove('loading');
    resultsDiv.innerHTML = '<div class="msg msg-error">Connection failed: ' + err.message + '</div>';
  }
}

// --- Vector Space Visualizer ---
function renderVisualizer(allPoints, queryPoint, results) {
  const container = document.getElementById('vizContainer');
  if (!allPoints || allPoints.length === 0) {
    container.innerHTML = '<div class="msg" style="padding:1rem">No points to plot</div>';
    return;
  }

  // Find bounds for scale
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  allPoints.forEach(p => {
    if (p.x < minX) minX = p.x; if (p.x > maxX) maxX = p.x;
    if (p.y < minY) minY = p.y; if (p.y > maxY) maxY = p.y;
  });
  
  // Include query point in bounds
  if (queryPoint && queryPoint.length === 2) {
    if (queryPoint[0] < minX) minX = queryPoint[0];
    if (queryPoint[0] > maxX) maxX = queryPoint[0];
    if (queryPoint[1] < minY) minY = queryPoint[1];
    if (queryPoint[1] > maxY) maxY = queryPoint[1];
  }

  // Padding
  const padX = (maxX - minX) * 0.1 || 0.1;
  const padY = (maxY - minY) * 0.1 || 0.1;
  minX -= padX; maxX += padX;
  minY -= padY; maxY += padY;

  const w = 300; const h = 300; // viewBox size

  function scaleX(val) { return ((val - minX) / (maxX - minX)) * w; }
  function scaleY(val) { return h - (((val - minY) / (maxY - minY)) * h); }

  let svg = '<svg viewBox="0 0 ' + w + ' ' + h + '">';
  
  // Draw faint bucket lines (simulated hyperplanes)
  svg += '<line x1="0" y1="150" x2="300" y2="150" class="viz-bucket"/>';
  svg += '<line x1="150" y1="0" x2="150" y2="300" class="viz-bucket"/>';

  // Draw background points
  allPoints.forEach(p => {
    const sx = scaleX(p.x);
    const sy = scaleY(p.y);
    svg += '<circle cx="'+sx+'" cy="'+sy+'" r="2.5" class="viz-point" id="pt-'+p.id+'" />';
  });

  // Draw Query & Traces if available
  if (queryPoint && queryPoint.length === 2) {
    const qx = scaleX(queryPoint[0]);
    const qy = scaleY(queryPoint[1]);
    
    // Traces to results
    if (results && results.length > 0) {
      results.forEach((r, i) => {
        const pt = allPoints.find(p => p.id === r.doc_id);
        if (pt) {
          const rx = scaleX(pt.x);
          const ry = scaleY(pt.y);
          svg += '<line x1="'+qx+'" y1="'+qy+'" x2="'+rx+'" y2="'+ry+'" class="viz-trace" style="animation-delay: '+(i*0.05)+'s" />';
        }
      });
    }

    // Query Point
    svg += '<circle cx="'+qx+'" cy="'+qy+'" r="3" class="viz-query" />';
  }

  svg += '</svg>';
  container.innerHTML = svg;

  // Add ambient drift (respects reduced motion via CSS)
  setTimeout(() => {
    document.querySelectorAll('.viz-point').forEach(el => {
      const dx = (Math.random() - 0.5) * 6;
      const dy = (Math.random() - 0.5) * 6;
      el.style.transform = 'translate(' + dx + 'px, ' + dy + 'px)';
    });
  }, 100);
}

// --- Stats Polling (WAL Sparkline) ---
let walHistory = Array(20).fill(0);
let lastWALSize = 0;

async function pollStats() {
  try {
    const res = await fetch('/api/stats');
    const data = await res.json();
    
    document.getElementById('statSSTables').textContent = data.storage_sstable_count || 0;
    document.getElementById('statMemtable').textContent = data.storage_memtable_size_bytes ? (data.storage_memtable_size_bytes / 1024).toFixed(1) + ' KB' : '0 KB';
    document.getElementById('statDocs').textContent = data.document_count || 0;
    document.getElementById('statVocab').textContent = data.vocabulary_size || 0;

    // WAL Sparkline calculation
    const currentWal = data.storage_wal_size_bytes || 0;
    let diff = currentWal - lastWALSize;
    if (diff < 0) diff = currentWal; // WAL truncated/reset
    lastWALSize = currentWal;

    walHistory.push(diff);
    walHistory.shift();

    // Render Sparkline
    const maxVal = Math.max(...walHistory, 1024); // at least 1KB scale
    let pathD = 'M0,30 ';
    walHistory.forEach((v, i) => {
      const x = (i / 19) * 100;
      const y = 30 - ((v / maxVal) * 28);
      pathD += 'L' + x + ',' + y + ' ';
    });
    document.querySelector('.spark-path').setAttribute('d', pathD);

  } catch (err) {
    // silently fail polling
  }
}

// Start polling
setInterval(pollStats, 1000);
pollStats();

// Utility
function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
</script>
</body>
</html>`
