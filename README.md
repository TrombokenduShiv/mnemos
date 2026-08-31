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
- Build 1 SHA256: `2e09b10df78652747316f5fa87f96305390f122c7adcd3bcefd42a69bc1306a5`
- Build 2 SHA256: `2e09b10df78652747316f5fa87f96305390f122c7adcd3bcefd42a69bc1306a5`

## Usage

### 1. Ingest Documents

Point Mnemos at a directory containing text files (`.txt`, `.md`, etc.). It will index them, train the tokenizer, compute embeddings, and build the search indices.

```bash
./mnemos.exe ingest path/to/your/documents
```

### 2. Start the Server

Launch the local HTTP interface.

```bash
./mnemos.exe serve
```

Then open `http://localhost:8080` in your browser.

### 3. Command Line Query

You can also query directly from the terminal.

```bash
./mnemos.exe query "How does cryptography work?"
```

## Zero-Dependency Architecture

See [STDLIB.md](STDLIB.md) for a detailed breakdown of every external dependency that was replaced with a custom standard-library implementation.

## License

MIT License
