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
// Cinematic instrument-panel with 3D tilt cards, magnetic hover, cursor glow,
// parallax particles, scroll reveals, and premium micro-interactions.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mnemos — Semantic Memory Engine</title>
<meta name="description" content="A zero-dependency local semantic memory engine. Search your documents with AI-powered hybrid ranking.">
<style>
  @import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap');

  :root {
    --bg-void: #050810;
    --border-subtle: rgba(94, 234, 212, 0.06);
    --border-med: rgba(94, 234, 212, 0.14);
    --border-focus: rgba(94, 234, 212, 0.35);
    --accent: #5EEAD4;
    --accent-dim: rgba(94, 234, 212, 0.12);
    --amber: #FBBF24;
    --amber-dim: rgba(251, 191, 36, 0.12);
    --violet: #A78BFA;
    --violet-dim: rgba(167, 139, 250, 0.12);
    --rose: #FB7185;
    --emerald: #34D399;
    --ink-hi: #F0F4F8;
    --ink-md: #94A3B8;
    --ink-lo: #64748B;
    --font: 'Inter', system-ui, sans-serif;
    --mono: 'JetBrains Mono', ui-monospace, Consolas, monospace;
    --ease: cubic-bezier(0.16, 1, 0.3, 1);
    --spring: cubic-bezier(0.34, 1.56, 0.64, 1);
  }

  *{box-sizing:border-box;margin:0;padding:0}
  html{scroll-behavior:smooth}

  body {
    background: var(--bg-void);
    color: var(--ink-hi);
    font-family: var(--font);
    font-size: 15px;
    line-height: 1.6;
    min-height: 100vh;
    overflow-x: hidden;
    -webkit-font-smoothing: antialiased;
  }

  /* ── Cursor Glow ── */
  .cursor-glow {
    position: fixed;
    width: 600px; height: 600px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(94,234,212,0.06) 0%, transparent 70%);
    pointer-events: none;
    z-index: 0;
    transform: translate(-50%, -50%);
    transition: opacity 0.4s ease;
    will-change: left, top;
  }

  /* ── Canvas ── */
  #particleCanvas {
    position: fixed; top: 0; left: 0;
    width: 100vw; height: 100vh;
    z-index: 0; pointer-events: none;
  }

  /* ── Noise grain overlay ── */
  .grain {
    position: fixed; top: -50%; left: -50%;
    width: 200%; height: 200%;
    z-index: 9999; pointer-events: none;
    opacity: 0.025;
    background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
    background-repeat: repeat;
  }

  /* ── Layout ── */
  .app { position: relative; z-index: 1; display: flex; justify-content: center; padding: 2.5rem 2rem; min-height: 100vh; }
  .layout { display: grid; grid-template-columns: 1fr 380px; gap: 3.5rem; width: 100%; max-width: 1360px; }

  /* ── Smooth Scroll reveal ── */
  .reveal { opacity: 0; transform: translateY(40px); transition: opacity 1s var(--ease), transform 1s var(--ease); }
  .reveal.visible { opacity: 1; transform: translateY(0); }

  /* ── Header ── */
  .header { margin-bottom: 3.5rem; display: flex; align-items: center; gap: 1.1rem; }
  .logo {
    width: 48px; height: 48px;
    background: linear-gradient(135deg, var(--accent) 0%, var(--violet) 100%);
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    font-size: 1.4rem; font-weight: 800; color: var(--bg-void);
    box-shadow: 0 0 28px rgba(94,234,212,0.25);
  }
  .header-text h1 {
    font-size: 1.75rem; font-weight: 800; letter-spacing: -0.04em;
    background: linear-gradient(135deg, #fff 30%, var(--accent) 100%);
    -webkit-background-clip: text; -webkit-text-fill-color: transparent;
    background-clip: text; line-height: 1.15;
  }
  .header-text .sub {
    font-size: 0.72rem; color: var(--ink-lo); letter-spacing: 0.14em;
    text-transform: uppercase; font-weight: 600; margin-top: 2px;
  }

  /* ── Search (Fluid) ── */
  .search-wrap { position: relative; margin-bottom: 3.5rem; }
  .search-bar {
    display: flex; align-items: center;
    border-bottom: 2px solid var(--border-subtle);
    padding-bottom: 0.5rem;
    transition: border-color 0.3s ease;
  }
  .search-bar:focus-within { border-color: var(--accent); }
  .s-icon { padding: 0 1rem 0 0.5rem; color: var(--accent); display: flex; align-items: center; }
  .s-icon svg { width: 24px; height: 24px; }
  .q-input {
    flex: 1; background: transparent; border: none; color: var(--ink-hi);
    font-family: var(--font); font-size: 1.5rem; font-weight: 300; padding: 0.5rem 0; outline: none;
  }
  .q-input::placeholder { color: var(--ink-lo); font-weight: 300; }
  .s-btn {
    background: transparent; border: 1px solid var(--accent); color: var(--accent);
    padding: 0.6rem 1.4rem; margin-left: 1rem; border-radius: 4px;
    cursor: pointer; font-family: var(--mono); font-size: 0.8rem; letter-spacing: 0.1em;
    text-transform: uppercase; transition: all 0.2s var(--ease);
  }
  .s-btn:hover { background: var(--accent); color: var(--bg-void); }
  
  .s-progress {
    position: absolute; bottom: 0; left: 0; height: 2px;
    background: linear-gradient(90deg, var(--accent), var(--violet), var(--accent));
    background-size: 200% 100%; width: 0%; transition: width 0.5s var(--ease);
    animation: shimmer 1.5s ease infinite;
  }
  @keyframes shimmer { 0%{background-position:200% 0} 100%{background-position:-200% 0} }
  .search-wrap.loading .s-progress { width: 100%; }

  /* ── Results Meta ── */
  .r-meta { display: flex; align-items: center; gap: 1rem; margin-bottom: 2rem; padding-bottom: 1rem; }
  .r-count { font-size: 0.9rem; color: var(--ink-md); font-weight: 400; }
  .r-count strong { color: var(--accent); font-weight: 600; }
  .r-lat { font-family: var(--mono); font-size: 0.75rem; color: var(--ink-lo); }

  /* ── Result Items (Fluid) ── */
  .results { display: flex; flex-direction: column; gap: 3rem; }
  .r-card {
    position: relative; padding-left: 1.5rem; border-left: 2px solid transparent;
    transition: border-color 0.3s ease, transform 0.3s var(--ease);
    opacity: 0; transform: translateY(30px);
    animation: fadeUp 0.8s var(--ease) forwards;
  }
  .r-card:hover { border-left-color: var(--accent); transform: translateX(10px); }
  @keyframes fadeUp { to { opacity: 1; transform: translateY(0); } }

  .r-rank {
    position: absolute; left: -3rem; top: 0.2rem;
    font-family: var(--mono); font-size: 0.8rem; font-weight: 500; color: var(--ink-lo);
  }
  .r-title { font-size: 1.4rem; font-weight: 600; color: var(--ink-hi); margin-bottom: 0.3rem; letter-spacing: -0.02em; }
  .r-path { font-family: var(--mono); font-size: 0.8rem; color: var(--ink-lo); margin-bottom: 1rem; display: block; }
  .scores { display: flex; gap: 1rem; margin-bottom: 1rem; }
  .sb { font-family: var(--mono); font-size: 0.75rem; display: inline-flex; align-items: center; gap: 0.4rem; color: var(--ink-md); }
  .sd { width: 6px; height: 6px; border-radius: 50%; display: inline-block; }
  .sd.bm25 { background: var(--amber); } .sd.emb { background: var(--violet); } .sd.fus { background: var(--accent); }
  .r-snip { font-size: 1rem; color: var(--ink-md); line-height: 1.8; max-height: 5.4rem; overflow: hidden; display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; text-overflow: ellipsis; font-weight: 300; }

  /* ── Instrument Strip (Fluid) ── */
  .instr { display: flex; flex-direction: column; gap: 3.5rem; position: sticky; top: 3.5rem; }
  .pnl { padding: 0; }
  .pt {
    font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.15em;
    color: var(--ink-hi); font-weight: 600; margin-bottom: 1.5rem;
    display: flex; justify-content: space-between; align-items: center;
    border-bottom: 1px solid var(--border-subtle); padding-bottom: 0.5rem;
  }
  .live { display: inline-flex; align-items: center; gap: 0.4rem; }
  .live-dot { width: 6px; height: 6px; background: var(--emerald); border-radius: 50%; animation: pulse 2s infinite; }
  @keyframes pulse { 0%,100%{opacity:1;box-shadow:0 0 4px var(--emerald)} 50%{opacity:0.3;box-shadow:0 0 10px var(--emerald)} }

  /* ── Toggle Button ── */
  .tgl-btn {
    font-size: 0.65rem; color: var(--violet); cursor: pointer; user-select: none;
    border: 1px solid var(--border-subtle); padding: 0.2rem 0.5rem; border-radius: 4px;
    transition: all 0.2s ease;
  }
  .tgl-btn:hover { border-color: var(--violet); background: rgba(167, 139, 250, 0.1); }

  /* ── 3D Vector Visualizer ── */
  .viz-3d {
    width: 100%; aspect-ratio: 1; position: relative;
    perspective: 800px;
  }
  .viz-3d svg { width: 100%; height: 100%; transition: transform 0.6s var(--ease); }
  .viz-point { fill: var(--ink-lo); opacity: 0.4; transition: all 0.4s ease; }
  .viz-point:hover { opacity: 1; fill: var(--ink-hi); }
  .viz-bucket { stroke: var(--ink-lo); stroke-width: 0.5; stroke-dasharray: 2 8; opacity: 0.2; }
  .viz-query { fill: var(--accent); filter: url(#glow); animation: qPulse 2.5s ease-in-out infinite; }
  .viz-trace { stroke: url(#tGrad); stroke-width: 1.5; opacity: 0.5; stroke-dasharray: 500; stroke-dashoffset: 500; animation: tDraw 1.2s var(--ease) forwards; }
  @keyframes tDraw { to { stroke-dashoffset: 0; } }
  @keyframes qPulse { 0%,100%{r:4;opacity:1} 50%{r:8;opacity:0.6} }
  .viz-result { fill: var(--accent); opacity: 0.8; }

  /* ── Stats ── */
  .sg { display: flex; flex-direction: column; gap: 1.5rem; }
  .sc { display: flex; justify-content: space-between; align-items:baseline; }
  .sc .sl { font-size: 0.75rem; color: var(--ink-lo); text-transform: uppercase; letter-spacing: 0.1em; }
  .sc .sv { font-family: var(--mono); font-size: 1.2rem; font-weight: 500; color: var(--ink-hi); }
  .sc.ac .sv { color: var(--accent); }

  /* ── Sparkline ── */
  .spk { width: 100%; height: 60px; margin-top: -10px; }
  .spk svg { width: 100%; height: 100%; }
  .spk-fill { fill: url(#sGrad); opacity: 0.2; }
  .spk-line { fill: none; stroke: var(--amber); stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }

  /* ── Empty/Loading States ── */
  .empty { padding: 6rem 0; color: var(--ink-lo); max-width: 400px; }
  .empty h3 { font-size: 1.5rem; font-weight: 600; color: var(--ink-hi); margin-bottom: 1rem; letter-spacing: -0.02em; }
  .empty p { font-size: 1rem; line-height: 1.7; font-weight: 300; }
  .msg-err { color: var(--rose); padding: 1.5rem 0; font-size: 1rem; font-weight: 500; }
  .ldots { display: inline-flex; gap: 0.4rem; margin-bottom: 2rem; }
  .ldots span { width: 8px; height: 8px; background: var(--accent); border-radius: 50%; animation: bounce 1.4s ease-in-out infinite; }
  .ldots span:nth-child(2){animation-delay:.16s} .ldots span:nth-child(3){animation-delay:.32s}
  @keyframes bounce { 0%,80%,100%{transform:translateY(0);opacity:0.3} 40%{transform:translateY(-8px);opacity:1} }

  @media(max-width:1000px){
    .layout{grid-template-columns:1fr}
    .instr{position:static; margin-top: 4rem;}
  }
  .viz-tip {
    position: absolute;
    opacity: 0;
    pointer-events: none;
    background: var(--bg-panel);
    border: 1px solid var(--accent);
    padding: 0.75rem;
    border-radius: 6px;
    font-size: 0.85rem;
    z-index: 1000;
    transition: opacity 0.2s;
    backdrop-filter: blur(10px);
    box-shadow: 0 4px 12px rgba(0,0,0,0.5);
    color: var(--text-pri);
    white-space: nowrap;
  }
</style>
</head>
<body>

<div class="cursor-glow" id="cursorGlow"></div>
<canvas id="particleCanvas"></canvas>
<div class="grain"></div>

<div class="app">
<div class="layout">
  <main>
    <div class="header reveal">
      <div class="logo">M</div>
      <div class="header-text">
        <h1>Mnemos</h1>
        <div class="sub">Zero-Dependency Semantic Memory Engine</div>
      </div>
    </div>

    <div class="search-wrap reveal" id="searchWrap">
      <div class="search-bar">
        <div class="s-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg></div>
        <input type="text" class="q-input" id="qInput" placeholder="Search the semantic index..." autofocus>
        <button class="s-btn" id="sBtn" onclick="doQuery()">Search</button>
      </div>
      <div class="s-progress"></div>
    </div>

    <div id="results" class="results">
      <div class="empty reveal">
        <h3>Ready to Search</h3>
        <p>Type a natural-language question. The engine will retrieve documents using hybrid BM25 and semantic embedding ranking.</p>
      </div>
    </div>
  </main>

  <aside class="instr">
    <div class="pnl reveal">
      <div class="pt"><span>Vector Space</span><span class="tgl-btn" id="vizToggle" onclick="toggleViz()">3D PROJECTION</span></div>
      <div class="viz-3d" id="vizC">
        <svg viewBox="0 0 300 300">
          <defs>
            <filter id="glow"><feGaussianBlur stdDeviation="4" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
            <linearGradient id="tGrad" x1="0%" y1="0%" x2="100%" y2="0%"><stop offset="0%" stop-color="#5EEAD4"/><stop offset="100%" stop-color="#A78BFA"/></linearGradient>
          </defs>
          <line x1="0" y1="150" x2="300" y2="150" class="viz-bucket"/>
          <line x1="150" y1="0" x2="150" y2="300" class="viz-bucket"/>
          <text x="150" y="156" text-anchor="middle" fill="#64748B" font-size="12" font-family="Inter" font-weight="300">Awaiting query</text>
        </svg>
      </div>
    </div>

    <div class="pnl reveal" style="transition-delay: 0.1s">
      <div class="pt"><span>Telemetry</span><span class="live"><span class="live-dot"></span><span style="font-size:0.6rem;color:var(--emerald)">LIVE</span></span></div>
      <div class="sg">
        <div class="sc"><span class="sl">Documents</span><span class="sv" id="sDocs">—</span></div>
        <div class="sc"><span class="sl">Vocabulary</span><span class="sv" id="sVocab">—</span></div>
        <div class="sc"><span class="sl">SSTables</span><span class="sv" id="sSST">—</span></div>
        <div class="sc"><span class="sl">Memtable</span><span class="sv" id="sMem">—</span></div>
        <div class="sc ac"><span class="sl">Latency</span><span class="sv" id="sLat">—</span></div>
      </div>
    </div>

    <div class="pnl reveal" style="transition-delay: 0.2s">
      <div class="pt"><span>WAL Stream</span><span style="font-size:0.6rem;color:var(--amber)">SYNCING</span></div>
      <div class="spk">
        <svg id="walSvg" viewBox="0 0 200 48" preserveAspectRatio="none">
          <defs><linearGradient id="sGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="#FBBF24" stop-opacity="0.5"/><stop offset="100%" stop-color="#FBBF24" stop-opacity="0"/></linearGradient></defs>
          <path class="spk-fill" d="M0,48 L200,48"/>
          <path class="spk-line" d="M0,48 L200,48"/>
        </svg>
      </div>
    </div>
  </aside>
</div>
</div>

<script>
// ══════ Cursor Glow ══════
const glow = document.getElementById('cursorGlow');
let mx = 0, my = 0, gx = 0, gy = 0;
document.addEventListener('mousemove', e => { mx = e.clientX; my = e.clientY; });
function animGlow() {
  gx += (mx - gx) * 0.08; gy += (my - gy) * 0.08;
  glow.style.left = gx + 'px'; glow.style.top = gy + 'px';
  requestAnimationFrame(animGlow);
}
animGlow();

// ══════ Particles ══════
(function(){
  const c = document.getElementById('particleCanvas'), ctx = c.getContext('2d');
  const rm = window.matchMedia('(prefers-reduced-motion:reduce)').matches;
  let pts = [], W, H;
  function resize(){ W = c.width = innerWidth; H = c.height = innerHeight; }
  resize(); addEventListener('resize', resize);
  if(!rm){
    for(let i=0;i<70;i++) pts.push({x:Math.random()*W,y:Math.random()*H,vx:(Math.random()-0.5)*0.25,vy:(Math.random()-0.5)*0.25,r:Math.random()*1.4+0.4,a:Math.random()*0.12+0.02,d:Math.random()*2+1});
    function draw(){
      ctx.clearRect(0,0,W,H);
      const cmx = mx/W, cmy = my/H;
      for(const p of pts){
        const px = (cmx - 0.5) * p.d * 8, py = (cmy - 0.5) * p.d * 8;
        p.x += p.vx; p.y += p.vy;
        if(p.x<0)p.x=W; if(p.x>W)p.x=0; if(p.y<0)p.y=H; if(p.y>H)p.y=0;
        ctx.beginPath(); ctx.arc(p.x+px, p.y+py, p.r, 0, Math.PI*2);
        ctx.fillStyle='rgba(94,234,212,'+p.a+')'; ctx.fill();
      }
      for(let i=0;i<pts.length;i++){
        for(let j=i+1;j<pts.length;j++){
          const dx=pts[i].x-pts[j].x, dy=pts[i].y-pts[j].y, d=dx*dx+dy*dy;
          if(d<15000){ctx.beginPath();ctx.moveTo(pts[i].x,pts[i].y);ctx.lineTo(pts[j].x,pts[j].y);ctx.strokeStyle='rgba(94,234,212,'+(0.035*(1-d/15000))+')';ctx.lineWidth=0.5;ctx.stroke();}
        }
      }
      requestAnimationFrame(draw);
    }
    draw();
  }
})();

// ══════ Smooth Scroll Reveal ══════
const obs = new IntersectionObserver(es=>{es.forEach(e=>{if(e.isIntersecting)e.target.classList.add('visible')})},{threshold:0.1, rootMargin: '0px 0px -50px 0px'});
document.querySelectorAll('.reveal').forEach(el=>obs.observe(el));

// ══════ 2D/3D Viz Toggle ══════
let is3D = true;
function applyVizTransform() {
  const svg = vizC.querySelector('svg');
  if (!svg) return;
  if (is3D) {
    svg.style.transform = 'rotateY(-20deg) rotateX(15deg) scale(0.9)';
  } else {
    svg.style.transform = 'rotateY(0) rotateX(0) scale(1)';
  }
}

function toggleViz() {
  is3D = !is3D;
  document.getElementById('vizToggle').textContent = is3D ? '3D PROJECTION' : '2D PROJECTION';
  applyVizTransform();
}

const vizC = document.getElementById('vizC');
// Initial transform for 3D state
setTimeout(applyVizTransform, 100);

vizC.addEventListener('mousemove', e=>{
  if (!is3D) return;
  const r = vizC.getBoundingClientRect();
  const x = (e.clientX-r.left)/r.width - 0.5;
  const y = (e.clientY-r.top)/r.height - 0.5;
  const svg = vizC.querySelector('svg');
  if(svg) svg.style.transform = 'rotateY('+(x*30 - 15)+'deg) rotateX('+(-y*30 + 10)+'deg) scale(0.95)';
});
vizC.addEventListener('mouseleave', ()=>{
  applyVizTransform();
});

// ══════ Query ══════
const qInput = document.getElementById('qInput');
qInput.addEventListener('keydown', e=>{ if(e.key==='Enter') doQuery(); });

async function doQuery(){
  const q = qInput.value.trim();
  if(!q) return;
  const wrap = document.getElementById('searchWrap');
  wrap.classList.add('loading');
  const rDiv = document.getElementById('results');
  rDiv.innerHTML = '<div class="empty reveal"><div class="ldots"><span></span><span></span><span></span></div><h3>Searching</h3><p>Extracting semantics and matching vectors...</p></div>';
  setTimeout(() => rDiv.querySelector('.reveal').classList.add('visible'), 10);
  
  try {
    const res = await fetch('/api/query',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({q,k:10})});
    const data = await res.json();
    wrap.classList.remove('loading');
    if(data.error){ rDiv.innerHTML='<div class="msg-err">'+esc(data.error)+'</div>'; return; }
    if(!data.results||data.results.length===0){ rDiv.innerHTML='<div class="empty reveal"><h3>No Results</h3><p>Try different keywords or phrasing.</p></div>'; setTimeout(()=>rDiv.querySelector('.reveal').classList.add('visible'),10); return; }
    
    document.getElementById('sLat').textContent = data.latency_ms.toFixed(1)+' ms';
    let h = '<div class="r-meta reveal visible"><span class="r-count"><strong>'+data.results.length+'</strong> results</span><span class="r-lat">'+data.latency_ms.toFixed(1)+' ms</span></div>';
    
    data.results.forEach((r,i)=>{
      h+='<div class="r-card reveal" id="res-'+r.doc_id+'" style="animation-delay:'+(i*0.1)+'s">';
      h+='<div class="r-rank">#'+(i+1)+'</div>';
      h+='<div class="r-title">'+esc(r.title)+'</div>';
      h+='<span class="r-path">'+esc(r.path)+'</span>';
      h+='<div class="scores">';
      h+='<span class="sb"><span class="sd bm25"></span>BM25 '+r.bm25_score.toFixed(2)+'</span>';
      h+='<span class="sb"><span class="sd emb"></span>Embed '+r.embed_score.toFixed(2)+'</span>';
      h+='<span class="sb"><span class="sd fus"></span>Fused '+r.score.toFixed(2)+'</span>';
      h+='</div><div class="r-snip">'+esc(r.snippet)+'</div></div>';
    });
    rDiv.innerHTML = h;
    document.querySelectorAll('.reveal').forEach(el=>obs.observe(el));
    renderViz(data.all_points, data.query_point, data.results);
  } catch(err){
    wrap.classList.remove('loading');
    rDiv.innerHTML='<div class="msg-err">Connection failed: '+esc(err.message)+'</div>';
  }
}

// ══════ Visualizer ══════
let vizTip = null;

window.showNodeTooltip = function(e, title, score, id) {
  if(!vizTip) {
    vizTip = document.createElement('div');
    vizTip.className = 'viz-tip';
    document.body.appendChild(vizTip);
  }
  vizTip.innerHTML = '<strong>' + title + '</strong><br><span style="color:var(--text-sec);font-size:0.75rem">Match: ' + score + '</span><br><span style="color:var(--accent);font-size:0.7rem;margin-top:4px;display:inline-block">Click to view</span>';
  vizTip.style.left = (e.pageX + 15) + 'px';
  vizTip.style.top = (e.pageY + 15) + 'px';
  vizTip.style.opacity = 1;
};
window.hideNodeTooltip = function() {
  if(vizTip) vizTip.style.opacity = 0;
};
window.scrollToRes = function(id) {
  const el = document.getElementById('res-'+id);
  if(el) {
    el.scrollIntoView({behavior:'smooth', block:'center'});
    el.style.transition = 'border-color 0.3s';
    el.style.borderColor = 'var(--accent)';
    setTimeout(function(){el.style.borderColor='var(--bg-line)';}, 2000);
  }
};

function renderViz(pts, qp, res){
  const w=300, h=300, cx=150, cy=150;
  let s='<svg viewBox="0 0 '+w+' '+h+'">';
  s+='<defs><filter id="glow"><feGaussianBlur stdDeviation="4" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>';
  s+='<linearGradient id="tGrad" x1="0%" y1="0%" x2="100%" y2="0%"><stop offset="0%" stop-color="#5EEAD4"/><stop offset="100%" stop-color="#A78BFA"/></linearGradient></defs>';
  s+='<line x1="0" y1="150" x2="300" y2="150" class="viz-bucket"/><line x1="150" y1="0" x2="150" y2="300" class="viz-bucket"/>';
  
  const resMap = new Map();
  if(res) {
    res.forEach(function(r, i) {
      const angle = (i / res.length) * Math.PI * 2;
      let dist = 140 - (r.score * 40);
      if (dist < 30) dist = 30; if (dist > 140) dist = 140;
      const rx = cx + Math.cos(angle) * dist;
      const ry = cy + Math.sin(angle) * dist;
      resMap.set(r.doc_id, {rx: rx, ry: ry, title: r.title, score: r.score});
    });
  }

  if(qp && res) {
    res.forEach(function(r, i) {
      const node = resMap.get(r.doc_id);
      if (node) {
        const opacity = Math.min(1, 0.2 + (r.score * 0.3));
        const strokeW = 0.5 + (r.score * 0.5);
        s+='<line x1="'+cx+'" y1="'+cy+'" x2="'+node.rx+'" y2="'+node.ry+'" class="viz-trace" style="animation-delay:'+(i*0.1)+'s; stroke-width:'+strokeW+'; opacity:'+opacity+'"/>';
      }
    });
  }

  if(pts) {
    pts.forEach(function(p) {
      if(!resMap.has(p.id)){
        let hash = 0;
        for (let i = 0; i < p.id.length; i++) hash = Math.imul(31, hash) + p.id.charCodeAt(i) | 0;
        const r_angle = hash % (Math.PI*2);
        const r_dist = 60 + (Math.abs(hash) % 80);
        const px = cx + Math.cos(r_angle)*r_dist;
        const py = cy + Math.sin(r_angle)*r_dist;
        s+='<circle cx="'+px+'" cy="'+py+'" r="2" class="viz-point" style="opacity:0.3"/>';
      }
    });
  }

  if (resMap.size > 0) {
    resMap.forEach(function(node, id) {
      const escT = esc(node.title).replace(/'/g, "\\'");
      const scoreStr = node.score.toFixed(2);
      s+='<circle cx="'+node.rx+'" cy="'+node.ry+'" r="5" class="viz-result" filter="url(#glow)" onmousemove="showNodeTooltip(event, \''+escT+'\', \''+scoreStr+'\', \''+id+'\')" onmouseleave="hideNodeTooltip()" onclick="scrollToRes(\''+id+'\')" style="cursor:pointer"/>';
    });
  }

  if(qp) {
    s+='<circle cx="'+cx+'" cy="'+cy+'" r="6" class="viz-query" filter="url(#glow)"/>';
    s+='<text x="'+cx+'" y="'+(cy-12)+'" text-anchor="middle" fill="#5EEAD4" font-size="10" font-family="Inter" font-weight="600" filter="url(#glow)">QUERY</text>';
  } else {
    s+='<text x="150" y="156" text-anchor="middle" fill="#64748B" font-size="12" font-family="Inter" font-weight="300">Awaiting query</text>';
  }

  s+='</svg>';
  document.getElementById('vizC').innerHTML = s;
}

// ══════ Stats ══════
let wH=Array(40).fill(0), lW=0;
async function poll(){
  try{
    const r=await fetch('/api/stats'),d=await r.json();
    document.getElementById('sSST').textContent=d.storage_sstable_count||0;
    const mb=d.storage_memtable_size||0;
    document.getElementById('sMem').textContent=mb>1024?(mb/1024).toFixed(1)+' KB':mb+' B';
    document.getElementById('sDocs').textContent=d.document_count||0;
    document.getElementById('sVocab').textContent=d.vocabulary_size||0;
    const cw=d.storage_bytes_written||0;
    let df=Math.abs(cw-lW);if(lW===0)df=0;lW=cw;
    wH.push(df);wH.shift();
    const mv=Math.max(...wH,512);
    let lD='',fD='M0,48 ';
    wH.forEach((v,i)=>{const x=(i/(wH.length-1))*200,y=48-((v/mv)*42);lD+=(i===0?'M':'L')+x+','+y+' ';fD+='L'+x+','+y+' ';});
    fD+='L200,48 Z';
    document.querySelector('.spk-line').setAttribute('d',lD);
    document.querySelector('.spk-fill').setAttribute('d',fD);
  }catch(e){}
}
setInterval(poll,1500);poll();

function esc(t){const d=document.createElement('div');d.textContent=t;return d.innerHTML;}
</script>
</body>
</html>`

