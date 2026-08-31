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

// Mnemos is the interface the server uses to query the search engine.
type Mnemos interface {
	Search(query string, k int) ([]SearchResult, error)
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
	results, err := s.engine.Search(query, k)
	elapsed := time.Since(start)

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), 500)
		return
	}

	resp := struct {
		Query     string         `json:"query"`
		Results   []SearchResult `json:"results"`
		Count     int            `json:"count"`
		LatencyMs float64        `json:"latency_ms"`
	}{
		Query:     query,
		Results:   results,
		Count:     len(results),
		LatencyMs: float64(elapsed.Milliseconds()),
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
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Mnemos — Local Semantic Search</title>
<meta name="description" content="Private, zero-dependency semantic search over your local documents">
<style>
  :root {
    --bg: #0a0a0f;
    --surface: #12121a;
    --surface2: #1a1a2e;
    --border: #2a2a3e;
    --text: #e4e4f0;
    --text-dim: #8888a0;
    --accent: #7c5cff;
    --accent-glow: rgba(124, 92, 255, 0.3);
    --success: #4caf50;
    --highlight: #ffd54f;
    --radius: 12px;
    --font: 'Segoe UI', system-ui, -apple-system, sans-serif;
    --mono: 'Cascadia Code', 'Fira Code', 'JetBrains Mono', monospace;
  }

  * { margin: 0; padding: 0; box-sizing: border-box; }

  body {
    font-family: var(--font);
    background: var(--bg);
    color: var(--text);
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  .container {
    width: 100%;
    max-width: 900px;
    padding: 2rem 1.5rem;
  }

  header {
    text-align: center;
    margin-bottom: 3rem;
    animation: fadeIn 0.6s ease;
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(-10px); }
    to { opacity: 1; transform: translateY(0); }
  }

  h1 {
    font-size: 2.5rem;
    font-weight: 700;
    background: linear-gradient(135deg, var(--accent), #a78bfa, #c084fc);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    margin-bottom: 0.5rem;
    letter-spacing: -0.02em;
  }

  .subtitle {
    color: var(--text-dim);
    font-size: 0.95rem;
    letter-spacing: 0.02em;
  }

  .badge {
    display: inline-block;
    padding: 0.2rem 0.6rem;
    background: var(--surface2);
    border: 1px solid var(--border);
    border-radius: 20px;
    font-size: 0.75rem;
    color: var(--accent);
    margin-top: 0.75rem;
    font-family: var(--mono);
  }

  .search-box {
    position: relative;
    margin-bottom: 2rem;
    animation: fadeIn 0.8s ease;
  }

  .search-input {
    width: 100%;
    padding: 1rem 1.25rem;
    padding-right: 120px;
    background: var(--surface);
    border: 2px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    font-size: 1.05rem;
    font-family: var(--font);
    transition: all 0.3s ease;
    outline: none;
  }

  .search-input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 4px var(--accent-glow);
  }

  .search-input::placeholder { color: var(--text-dim); }

  .search-btn {
    position: absolute;
    right: 8px;
    top: 50%;
    transform: translateY(-50%);
    padding: 0.6rem 1.2rem;
    background: linear-gradient(135deg, var(--accent), #9b7bff);
    color: white;
    border: none;
    border-radius: 8px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
    font-family: var(--font);
  }

  .search-btn:hover {
    transform: translateY(-50%) scale(1.02);
    box-shadow: 0 4px 15px var(--accent-glow);
  }

  .search-btn:active { transform: translateY(-50%) scale(0.98); }

  .meta {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 0;
    margin-bottom: 1rem;
    color: var(--text-dim);
    font-size: 0.85rem;
  }

  .meta .latency {
    font-family: var(--mono);
    color: var(--success);
  }

  .results { animation: fadeIn 0.4s ease; }

  .result-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 1.25rem;
    margin-bottom: 1rem;
    transition: all 0.2s ease;
    cursor: default;
  }

  .result-card:hover {
    border-color: var(--accent);
    box-shadow: 0 4px 20px rgba(0,0,0,0.3);
    transform: translateY(-1px);
  }

  .result-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 0.75rem;
  }

  .result-title {
    font-size: 1.1rem;
    font-weight: 600;
    color: var(--text);
  }

  .result-rank {
    background: var(--accent);
    color: white;
    padding: 0.15rem 0.5rem;
    border-radius: 6px;
    font-size: 0.75rem;
    font-weight: 700;
    font-family: var(--mono);
    min-width: 28px;
    text-align: center;
  }

  .result-path {
    font-family: var(--mono);
    font-size: 0.8rem;
    color: var(--text-dim);
    margin-bottom: 0.75rem;
    word-break: break-all;
  }

  .result-snippet {
    font-size: 0.92rem;
    line-height: 1.6;
    color: var(--text);
    background: var(--surface2);
    padding: 0.75rem 1rem;
    border-radius: 8px;
    border-left: 3px solid var(--accent);
    margin-bottom: 0.75rem;
  }

  .result-scores {
    display: flex;
    gap: 1rem;
    font-family: var(--mono);
    font-size: 0.78rem;
    color: var(--text-dim);
  }

  .score-item {
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }

  .score-label { font-weight: 600; }

  .score-bar {
    width: 60px;
    height: 4px;
    background: var(--surface2);
    border-radius: 2px;
    overflow: hidden;
  }

  .score-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
    transition: width 0.5s ease;
  }

  .empty-state {
    text-align: center;
    padding: 4rem 2rem;
    color: var(--text-dim);
  }

  .empty-state .icon { font-size: 3rem; margin-bottom: 1rem; }
  .empty-state p { margin-bottom: 0.5rem; }

  .loading {
    text-align: center;
    padding: 2rem;
    color: var(--text-dim);
  }

  .spinner {
    display: inline-block;
    width: 24px;
    height: 24px;
    border: 3px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    margin-bottom: 0.5rem;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  footer {
    margin-top: auto;
    padding: 2rem;
    text-align: center;
    color: var(--text-dim);
    font-size: 0.8rem;
    border-top: 1px solid var(--border);
    width: 100%;
  }

  footer a { color: var(--accent); text-decoration: none; }
  footer a:hover { text-decoration: underline; }

  @media (max-width: 600px) {
    h1 { font-size: 1.8rem; }
    .search-input { padding-right: 100px; font-size: 0.95rem; }
    .result-scores { flex-wrap: wrap; gap: 0.5rem; }
  }
</style>
</head>
<body>
  <div class="container">
    <header>
      <h1>⚡ Mnemos</h1>
      <p class="subtitle">Private, zero-dependency semantic search over your local documents</p>
      <span class="badge">🔒 Fully offline · No telemetry · No cloud</span>
    </header>

    <div class="search-box">
      <input type="text" class="search-input" id="query"
             placeholder="Ask a question about your documents..."
             autocomplete="off" autofocus>
      <button class="search-btn" id="searchBtn" onclick="search()">Search</button>
    </div>

    <div id="results">
      <div class="empty-state">
        <div class="icon">🔍</div>
        <p>Type a natural-language question to search your documents</p>
        <p style="font-size: 0.85rem">Results are ranked using hybrid BM25 + semantic similarity fusion</p>
      </div>
    </div>
  </div>

  <footer>
    <p>Mnemos — Built from first principles with zero dependencies</p>
    <p>Storage: WAL + SSTable engine · NLP: BPE + PPMI/SVD · Index: SimHash LSH · Ranking: BM25 + RRF</p>
  </footer>

  <script>
    const input = document.getElementById('query');
    const resultsDiv = document.getElementById('results');

    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') search();
    });

    async function search() {
      const q = input.value.trim();
      if (!q) return;

      resultsDiv.innerHTML = '<div class="loading"><div class="spinner"></div><p>Searching...</p></div>';

      try {
        const res = await fetch('/api/query', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ q, k: 10 })
        });

        const data = await res.json();

        if (data.error) {
          resultsDiv.innerHTML = '<div class="empty-state"><div class="icon">⚠️</div><p>' + data.error + '</p></div>';
          return;
        }

        if (!data.results || data.results.length === 0) {
          resultsDiv.innerHTML = '<div class="empty-state"><div class="icon">📭</div><p>No results found for your query</p></div>';
          return;
        }

        let html = '<div class="meta"><span>' + data.count + ' result' + (data.count !== 1 ? 's' : '') + '</span><span class="latency">' + data.latency_ms.toFixed(1) + 'ms</span></div>';
        html += '<div class="results">';

        const maxScore = Math.max(...data.results.map(r => r.score));
        const maxBM25 = Math.max(...data.results.map(r => r.bm25_score || 0), 0.001);
        const maxEmbed = Math.max(...data.results.map(r => r.embed_score || 0), 0.001);

        data.results.forEach((r, i) => {
          const fusedPct = maxScore > 0 ? (r.score / maxScore * 100) : 0;
          const bm25Pct = maxBM25 > 0 ? ((r.bm25_score || 0) / maxBM25 * 100) : 0;
          const embedPct = maxEmbed > 0 ? ((r.embed_score || 0) / maxEmbed * 100) : 0;

          html += '<div class="result-card">' +
            '<div class="result-header">' +
              '<span class="result-title">' + escapeHtml(r.title || r.doc_id) + '</span>' +
              '<span class="result-rank">#' + (i + 1) + '</span>' +
            '</div>' +
            '<div class="result-path">' + escapeHtml(r.path || '') + '</div>' +
            '<div class="result-snippet">' + escapeHtml(r.snippet || '') + '</div>' +
            '<div class="result-scores">' +
              '<div class="score-item"><span class="score-label">Fused:</span>' + r.score.toFixed(4) + ' <div class="score-bar"><div class="score-fill" style="width:' + fusedPct + '%"></div></div></div>' +
              '<div class="score-item"><span class="score-label">BM25:</span>' + (r.bm25_score || 0).toFixed(4) + ' <div class="score-bar"><div class="score-fill" style="width:' + bm25Pct + '%"></div></div></div>' +
              '<div class="score-item"><span class="score-label">Embed:</span>' + (r.embed_score || 0).toFixed(4) + ' <div class="score-bar"><div class="score-fill" style="width:' + embedPct + '%"></div></div></div>' +
            '</div>' +
          '</div>';
        });

        html += '</div>';
        resultsDiv.innerHTML = html;
      } catch (err) {
        resultsDiv.innerHTML = '<div class="empty-state"><div class="icon">💥</div><p>Error: ' + err.message + '</p></div>';
      }
    }

    function escapeHtml(text) {
      const div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    }

    // Load stats
    fetch('/api/stats').then(r => r.json()).then(stats => {
      if (stats.document_count) {
        document.querySelector('.subtitle').textContent +=
          ' · ' + stats.document_count + ' documents indexed';
      }
    }).catch(() => {});
  </script>
</body>
</html>`
