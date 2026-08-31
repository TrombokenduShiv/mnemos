// Package mnemos is a Zero-Dependency Local Semantic Memory Engine.
//
// Mnemos provides fully offline, private semantic search over local documents without
// relying on a single external library or framework. Every single component—from
// the storage engine and data structures to the word embeddings and machine learning
// algorithms—is built from first principles using solely the Go standard library.
//
// # Features & Architecture
//
// Mnemos is fully self-contained and offers a state-of-the-art retrieval pipeline.
//
//   - LSM-Tree Storage Engine: A custom implementation of a Log-Structured Merge-Tree
//     with Write-Ahead Log (WAL), Memtable, SSTables, and background compaction.
//   - BPE Tokenizer: A Byte-Pair Encoding tokenizer trained from scratch on your local corpus.
//   - PPMI + SVD Embeddings: Distributional semantics generated using pure linear algebra.
//   - SimHash LSH Index: Locality-sensitive hashing for approximate nearest-neighbor search.
//   - Hybrid Ranking & Machine Learning: BM25 keyword scoring fused with semantic vector
//     similarity via Reciprocal Rank Fusion (RRF). A Deep Learning MLP Ranker then reranks results.
//   - TextRank Summarization: Extractive text summarization powered by PageRank-style graph algorithms.
//
// # The Cyberpunk Terminal User Interface (TUI)
//
// Mnemos features a stunning, custom-built cyberpunk-themed Terminal User Interface, operating
// completely without external TUI libraries like bubbletea or tview. It features dynamic vector
// space visualization, live telemetry (document count, vocabulary, SSTables, latency), and
// an interactive natural language search.
//
// # Ingesting Your Own Documents
//
// To make the engine work with your own documents on your device:
//  1. Create a new folder (e.g., my_documents/).
//  2. Drop your personal .txt, .md, or .pdf files into that folder.
//  3. Run the ingest command pointing to your folder:
//     ./mnemos.exe ingest my_documents --data-dir .mnemos --merges 8000 --dimensions 100
//
// # Zero-Dependency Disclosure
//
// Mnemos was constructed under the strict constraints of a Zero-Dependency hackathon.
//   - NO external databases (SQLite, Postgres).
//   - NO external search indices (ElasticSearch, Lucene).
//   - NO machine learning frameworks (PyTorch, TensorFlow).
//   - NO third-party math/linear algebra libraries.
//   - NO terminal UI frameworks.
//
// Everything is natively implemented in Go.
package mnemos
