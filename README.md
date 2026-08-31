# Mnemos

A Zero-Dependency Local Semantic Memory Engine, built for the **Zero Dependency | 72-Hour Hackathon** (Track D — Data & Storage).

Mnemos provides fully offline, private semantic search over local documents without relying on a single external library or framework. Every single component—from the storage engine and data structures to the word embeddings and machine learning algorithms—is built from first principles using solely the Go standard library. There is absolutely no `github.com/...` in our `go.mod` file.

## 🚀 Features & Architecture

Mnemos is fully self-contained and offers a state-of-the-art retrieval pipeline. 

### Core Components
- **LSM-Tree Storage Engine**: A custom implementation of a Log-Structured Merge-Tree. Features a Write-Ahead Log (WAL) for durability, an in-memory Memtable, immutable SSTables, and background compaction.
- **BPE Tokenizer**: A Byte-Pair Encoding tokenizer trained from scratch on your local corpus.
- **PPMI + SVD Embeddings**: Distributional semantics generated using Positive Pointwise Mutual Information and Singular Value Decomposition via pure linear algebra.
- **SimHash LSH Index**: Locality-sensitive hashing that enables blazing-fast approximate nearest-neighbor searches in constant time.
- **Hybrid Ranking & Machine Learning**: Combines traditional BM25 keyword scoring with semantic vector similarity, fused using Reciprocal Rank Fusion (RRF). Finally, a Deep Learning MLP (Multilayer Perceptron) Ranker trained via Gradient Descent reranks the results to provide the most relevant matches.
- **TextRank Summarization**: Extractive text summarization powered by PageRank-style graph algorithms.

## 💻 The Cyberpunk Terminal User Interface (TUI)

Mnemos features a stunning, custom-built cyberpunk-themed Terminal User Interface, operating completely without external TUI libraries like `bubbletea` or `tview`. 

*(Replace the paths below with your actual uploaded image files, e.g., `assets/tui_ready.png`)*

![TUI Interface - Ready to Search](./demo%20images/tui_ready.png)
![TUI Interface - Query Results & Vector Space](./demo%20images/tui_results.png)

**TUI Features include:**
- **Vector Space Visualization**: Watch your query and document embeddings dynamically plotted in a 2D space right in your terminal.
- **Live Telemetry Dashboard**: Real-time stats showing Document Count, Vocabulary Size, SSTable counts, Memtable size, and query latency (resolving complex semantic queries in as little as `1ms`).
- **Interactive Search**: Type natural language queries (e.g., "Machine Learning") and immediately get highlighted, relevant document snippets with individual BM25, Embedding, and Fused scores.

## 🛠️ Installation

Ensure you have Go 1.20+ installed.

```bash
git clone https://github.com/TrombokenduShiv/mnemos.git
cd mnemos
go build -trimpath -ldflags="-s -w" -o mnemos.exe ./cmd/mnemos
```

**Reproducible Build Proof:**
Building the exact same commit twice yields a byte-identical binary.
- Build 1 SHA256: `14c43d720a0a1c51b2db226ea62b57e209550bc549cb7e83dd2a383c39080691`
- Build 2 SHA256: `14c43d720a0a1c51b2db226ea62b57e209550bc549cb7e83dd2a383c39080691`

## 📖 Usage Guide

### 1. Build the Engine
```bash
go build -o mnemos.exe ./cmd/mnemos
```

### 2. Ingest Documents
Point the engine to your corpus (Markdown, TXT, PDFs). The engine will read, tokenize, embed, and index your data from scratch.

By default, you can test with the provided sample data:
```bash
./mnemos.exe ingest testdata/corpus --data-dir .mnemos --merges 8000 --dimensions 100
```

**Using Your Own Data (Bring Your Own Corpus):**
To make the engine work with your own documents on your device:
1. Create a new folder (e.g., `my_documents/`).
2. Drop your personal `.txt`, `.md`, or `.pdf` files into that folder.
3. Run the ingest command pointing to your folder:
   ```bash
   ./mnemos.exe ingest my_documents --data-dir .mnemos --merges 8000 --dimensions 100
   ```
*(Note: You only need to ingest once, or whenever you add new files to the directory!)*

*During ingestion, the engine natively trains the BPE tokenizer, builds the BM25 index, computes PPMI/SVD embeddings, builds the SimHash LSH index, and trains the Deep Learning MLP ranker.*

![Ingestion CLI Output](./demo%20images/cli_ingest.png)

### 3. Engine Statistics
View detailed metrics and storage telemetry.
```bash
./mnemos.exe stats
```

![Engine Statistics Output](./demo%20images/cli_stats.png)

### 4. Search and Retrieve
Launch the cyberpunk TUI for an interactive experience:
```bash
./mnemos.exe tui
```
Alternatively, use the web interface or CLI:
```bash
./mnemos.exe web       # http://localhost:8080
./mnemos.exe query "how do LSM trees work?"
```

## 🛡️ Zero-Dependency Disclosure

Mnemos was constructed under the strict constraints of a Zero-Dependency hackathon. 
- **NO** external databases (SQLite, Postgres).
- **NO** external search indices (ElasticSearch, Lucene).
- **NO** machine learning frameworks (PyTorch, TensorFlow).
- **NO** third-party math/linear algebra libraries.
- **NO** terminal UI frameworks.

Everything is natively implemented in Go. See [STDLIB.md](STDLIB.md) and [DEPENDENCY_PROOF.md](DEPENDENCY_PROOF.md) for a detailed breakdown of every standard-library implementation.

## 📜 License
MIT License
