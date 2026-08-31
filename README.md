# Mnemos

A Zero-Dependency Local Semantic Memory Engine, built for the **Zero Dependency | 72-Hour Hackathon** (Track D — Data & Storage).

Mnemos provides fully offline, private semantic search over local documents without relying on a single external library or framework. Everything from the storage engine to the word embeddings is built from first principles using only the Go standard library.

## Features

- **LSM-Tree Storage Engine**: Custom WAL, Memtable, SSTable, and Compaction.
- **BPE Tokenizer**: Trained from scratch on your corpus.
- **PPMI + SVD Embeddings**: Distributional semantics using pure linear algebra.
- **SimHash LSH Index**: Locality-sensitive hashing for approximate nearest-neighbor search.
- **Hybrid Ranking**: BM25 keyword scoring fused with semantic similarity via Reciprocal Rank Fusion (RRF).
- **TextRank Summarization**: Extractive snippets using PageRank-style graph algorithms.
- **Embedded UI**: A sleek, dark-themed vanilla HTML/JS interface served over a native HTTP server.

## Installation

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

## Usage

# 1. Build the engine
go build -o mnemos.exe ./cmd/mnemos

# 2. Ingest documents (PDFs, Markdown, TXT)
./mnemos.exe ingest testdata/corpus

# 3. Launch the cyberpunk Terminal User Interface!
./mnemos.exe tui

# 4. Or launch the web interface (http://localhost:8080)
./mnemos.exe serve

# 5. CLI Query (with JSON support)
./mnemos.exe query "how do LSM trees work?"

## Zero-Dependency Architecture

See [STDLIB.md](STDLIB.md) for a detailed breakdown of every external dependency that was replaced with a custom standard-library implementation.

## License

MIT License
