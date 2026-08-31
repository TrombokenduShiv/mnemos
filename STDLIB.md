# STDLIB — Zero-Dependency Justification

This document details every external dependency that was replaced with a custom, first-principles implementation using only the Go standard library, meeting the core requirement of the **Zero Dependency** event.

## 1. Storage Engine
**Replaced:** `LevelDB`, `RocksDB`, `pebble`, `badger`
**Implementation:** `mnemos/internal/storage`
Built a complete LSM-tree storage engine from scratch, including:
- Write-Ahead Log (WAL) with CRC32 integrity checks and crash recovery
- Memtable (in-memory sorted KV store) with freeze/flush semantics
- Immutable SSTables (Sorted String Tables) with sparse indexing
- Background k-way merge compaction using `container/heap`
- Atomic manifest updates (`MANIFEST.json`)

## 2. Tokenizer
**Replaced:** HuggingFace `tokenizers`, `tiktoken`, `sentencepiece`
**Implementation:** `mnemos/internal/tokenizer`
Built a Byte-Pair Encoding (BPE) tokenizer from scratch, including:
- Corpus frequency analysis and pair merging
- Custom vocabulary generation and serialization
- Encoding (text → IDs) and decoding (IDs → text)

## 3. Word Embeddings
**Replaced:** `sentence-transformers`, `gensim`, `scikit-learn`
**Implementation:** `mnemos/internal/embed`
Built a distributional semantics engine using linear algebra:
- Co-occurrence matrix building with distance weighting
- Positive Pointwise Mutual Information (PPMI) computation
- Power-Iteration SVD (Singular Value Decomposition) to reduce dimensionality
- Pure math implementation — no matrix libraries used

## 4. Approximate Nearest Neighbor (ANN) Index
**Replaced:** `FAISS`, `Annoy`, `hnswlib`
**Implementation:** `mnemos/internal/index`
Built a Locality-Sensitive Hashing (LSH) index:
- Random hyperplane generation for cosine similarity
- SimHash signature computation
- Prefix-based bucketing for sub-linear search time

## 5. Keyword Ranking
**Replaced:** `Elasticsearch`, `rank_bm25` (PyPI)
**Implementation:** `mnemos/internal/rank`
Built a BM25 scoring engine:
- Custom inverted index
- TF-IDF with length normalization
- Reciprocal Rank Fusion (RRF) for hybrid search combination

## 6. Extractive Summarization
**Replaced:** `sumy`, `nltk`
**Implementation:** `mnemos/internal/summarize`
Built a TextRank summarizer:
- Rule-based sentence boundary detection
- Sentence similarity graph construction
- PageRank-style centrality scoring using power iteration

## 7. Web Server & Routing
**Replaced:** `Gin`, `Echo`, `Fiber`, `Express`
**Implementation:** `mnemos/internal/server`
Built a complete HTTP/1.1 JSON API and static file server using only `net/http` ServeMux.

## 8. Frontend UI & Styling
**Replaced:** `React`, `Vue`, `TailwindCSS`, `Bootstrap`
**Implementation:** `mnemos/internal/server/server.go` (embedded)
Built a responsive, dark-themed instrument panel UI using vanilla HTML/CSS/JS in a single embedded string, completely offline with no CDNs.

## 8.1. Data Visualization
**Replaced:** `D3.js`, `Chart.js`
**Implementation:** `mnemos/internal/server/server.go` (embedded SVG)
Built a live 2D vector-space visualizer using pure vanilla JS and inline SVG manipulation via the DOM API. Uses math computed natively on the backend (the first two dimensions of power-iteration SVD embeddings) to dynamically render points and traces.

## 9. Command Line Interface
**Replaced:** `cobra`, `urfave/cli`
**Implementation:** `mnemos/cmd/mnemos`
Built a multi-command CLI using only the standard `flag` package.

## 10. Terminal User Interface (TUI)
**Replaced:** `charmbracelet/bubbletea`, `tcell`, `ncurses`
**Implementation:** `mnemos/internal/tui`
Built a 30 FPS double-buffered TUI from scratch using:
- Raw ANSI True Color (24-bit RGB) escape sequences for styling and drawing.
- Standard `bytes.Buffer` for atomic frame rendering to eliminate flicker.
- Direct `syscall` API calls to set the Windows console to raw mode (`stty` via `os/exec` on Unix) to capture realtime keystrokes without `x/sys` or `x/term`.

## 11. Configuration Management
**Replaced:** `viper`, `godotenv`
**Implementation:** Native JSON unmarshaling and struct defaults.

## 11. Logging & Telemetry
**Replaced:** `zap`, `logrus`, `zerolog`
**Implementation:** Standard `log` and `fmt` printing to `os.Stderr`.
