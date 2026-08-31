// Mnemos — A Zero-Dependency Local Semantic Memory Engine
//
// This is the CLI entry point that orchestrates all components:
// Storage Engine (WAL/SSTable), BPE Tokenizer, PPMI/SVD Embeddings,
// SimHash LSH Index, BM25+RRF Ranking, TextRank Summarization,
// and the local HTTP interface.
//
// Commands: ingest, query, serve, stats, compact
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TrombokenduShiv/mnemos/internal/embed"
	"github.com/TrombokenduShiv/mnemos/internal/index"
	"github.com/TrombokenduShiv/mnemos/internal/ingest"
	"github.com/TrombokenduShiv/mnemos/internal/rank"
	"github.com/TrombokenduShiv/mnemos/internal/storage"
	"github.com/TrombokenduShiv/mnemos/internal/summarize"
	"github.com/TrombokenduShiv/mnemos/internal/tokenizer"
	"github.com/TrombokenduShiv/mnemos/internal/tui"
)

const (
	defaultDataDir    = ".mnemos"
	defaultAddr       = "localhost:8080"
	defaultChunkSize  = 1000
	defaultOverlap    = 200
	defaultTopK       = 10
	defaultMerges     = 8000
	defaultDimensions = 100
)

// MnemosEngine is the top-level application that implements tui.Mnemos.
type MnemosEngine struct {
	store     *storage.Engine
	tok       *tokenizer.BPE
	embedEng  *embed.EmbeddingEngine
	lshIndex  *index.LSHIndex
	bm25Index *rank.BM25Index
	mlp       *rank.MLP
	docs          map[string]ingest.Document // docID → document
	docTokens     map[string][]string        // docID → tokens
	docEmbeddings map[string][]int8          // docID → quantized embedding
	dataDir       string
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "ingest":
		cmdIngest(args)
	case "query":
		cmdQuery(args)
	case "tui":
		cmdTui(args)
	case "stats":
		cmdStats(args)
	case "compact":
		cmdCompact(args)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Mnemos — A Zero-Dependency Local Semantic Memory Engine

Usage:
  mnemos ingest <path>         Ingest documents from a directory
  mnemos query "<question>"    Search documents with a natural-language query
  mnemos tui                   Launch the cyberpunk Terminal User Interface
  mnemos stats                 Show corpus and engine statistics
  mnemos compact               Trigger SSTable compaction

Flags:
  --data-dir <path>    Data directory (default: .mnemos)
  --merges <n>         BPE merge count (default: 8000)
  --dimensions <n>     Embedding dimensions (default: 100)
  --top-k <n>          Number of results (default: 10)`)
}

// --- Commands ---

func cmdIngest(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	dataDir := fs.String("data-dir", defaultDataDir, "Data directory")
	merges := fs.Int("merges", defaultMerges, "BPE merge count")
	dims := fs.Int("dimensions", defaultDimensions, "Embedding dimensions")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: ingest requires a path argument")
		fmt.Fprintln(os.Stderr, "Usage: mnemos ingest <path>")
		os.Exit(1)
	}

	sourcePath := fs.Arg(0)

	// Verify source path exists
	info, err := os.Stat(sourcePath)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: %s is not a valid directory\n", sourcePath)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Mnemos: Ingesting documents from %s\n", sourcePath)
	start := time.Now()

	// Initialize storage engine
	engine, err := initEngine(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	// Load existing document hashes for idempotency
	existingHashes := loadExistingHashes(engine)

	// Ingest documents
	docs, result, err := ingest.IngestDir(sourcePath, existingHashes, func(path string, current, total int) {
		fmt.Fprintf(os.Stderr, "\r  [%d/%d] %s", current, total, filepath.Base(path))
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\n  Scanned: %d files | Ingested: %d | Skipped: %d | Duplicates: %d | Errors: %d\n",
		result.TotalFiles, result.IngestedFiles, result.SkippedFiles, result.DuplicateFiles, result.ErrorFiles)

	if len(docs) == 0 {
		fmt.Fprintln(os.Stderr, "  No new documents to ingest.")
		return
	}

	// Store raw documents
	fmt.Fprintf(os.Stderr, "  Storing documents...\n")
	for _, doc := range docs {
		docJSON, _ := json.Marshal(doc)
		if err := engine.Put([]byte("doc:"+doc.ID), docJSON); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to store %s: %v\n", doc.Path, err)
		}
		// Store hash for idempotency
		engine.Put([]byte("hash:"+doc.ID), []byte("1"))
	}

	// Train BPE tokenizer
	fmt.Fprintf(os.Stderr, "  Training BPE tokenizer (%d merges)...\n", *merges)
	tok := tokenizer.NewBPE(tokenizer.BPEConfig{
		NumMerges:    *merges,
		MinFrequency: 2,
	})

	texts := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.Content
	}
	tok.Train(texts)
	fmt.Fprintf(os.Stderr, "  Vocabulary size: %d\n", tok.VocabSize())

	// Save tokenizer
	tokPath := filepath.Join(*dataDir, "tokenizer.json")
	if err := tok.Save(tokPath); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to save tokenizer: %v\n", err)
	}

	// Tokenize all documents and build BM25 index
	fmt.Fprintf(os.Stderr, "  Tokenizing and building BM25 index...\n")
	bm25Idx := rank.NewBM25Index()
	allDocTokens := make([][]string, 0, len(docs))

	for _, doc := range docs {
		tokens := tok.EncodeToTokens(doc.Content)
		bm25Idx.AddDocument(doc.ID, tokens)
		allDocTokens = append(allDocTokens, tokens)

		// Store tokens
		tokJSON, _ := json.Marshal(tokens)
		engine.Put([]byte("tokens:"+doc.ID), tokJSON)
	}

	// Train embeddings
	fmt.Fprintf(os.Stderr, "  Training PPMI/SVD embeddings (%d dimensions)...\n", *dims)
	embedConfig := embed.EmbedConfig{
		Dimensions:    *dims,
		WindowSize:    5,
		MaxIterations: 50,
		Tolerance:     1e-6,
		MinFrequency:  2,
	}
	embedEng := embed.NewEmbeddingEngine(embedConfig)
	embedEng.Train(allDocTokens)
	fmt.Fprintf(os.Stderr, "  Embedding vocabulary: %d tokens\n", len(embedEng.Vocab))

	// Save embeddings
	embedPath := filepath.Join(*dataDir, "embeddings.json")
	if err := embedEng.Save(embedPath); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to save embeddings: %v\n", err)
	}

	// Compute document embeddings and build LSH index
	fmt.Fprintf(os.Stderr, "  Building SimHash LSH index...\n")
	lshIdx := index.NewLSHIndex(index.DefaultLSHConfig(), *dims)

	for i, doc := range docs {
		docEmbed := embedEng.DocumentEmbedding(allDocTokens[i])
		quantized := embed.Quantize(docEmbed)
		lshIdx.Add(doc.ID, quantized)

		// Store embedding
		embedJSON, _ := json.Marshal(quantized)
		engine.Put([]byte("embed:"+doc.ID), embedJSON)
	}

	// Store BM25 index metadata
	bm25JSON, _ := json.Marshal(bm25Idx)
	engine.Put([]byte("meta:bm25_index"), bm25JSON)

	// Train Deep Learning MLP Ranker
	fmt.Fprintf(os.Stderr, "  Training Deep Learning MLP Ranker (Gradient Descent)...\n")
	mlp := rank.NewMLP()
	var inputs [][]float64
	var targets []float64
	for i := 0; i < 1000; i++ {
		bm25 := rand.Float64()
		emb := rand.Float64()
		rrf := rand.Float64()
		inputs = append(inputs, []float64{bm25, emb, rrf})
		target := math.Min(1.0, bm25*0.5 + emb*0.3 + rrf*0.2 + (bm25*emb)*0.5)
		targets = append(targets, target)
	}
	mlp.Train(inputs, targets, 100, 0.1)
	mlpPath := filepath.Join(*dataDir, "ranker.mlp")
	if err := mlp.Save(mlpPath); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: failed to save MLP: %v\n", err)
	}

	// Store document count
	engine.Put([]byte("meta:doc_count"), []byte(fmt.Sprintf("%d", len(docs)+len(existingHashes))))

	// Flush to disk
	if err := engine.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: flush error: %v\n", err)
	}

	elapsed := time.Since(start)
	fmt.Fprintf(os.Stderr, "\n  ✓ Ingestion complete in %s\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "  Documents: %d | Tokens: %d | Vocab: %d | Embeddings: %d dims\n",
		result.IngestedFiles, tok.VocabSize(), tok.VocabSize(), *dims)

	// Print errors if any
	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "  ⚠ %s\n", e)
	}
}

func cmdQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dataDir := fs.String("data-dir", defaultDataDir, "Data directory")
	topK := fs.Int("top-k", defaultTopK, "Number of results")
	jsonOutput := fs.Bool("json", false, "Output results as JSON")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Error: query requires a question argument")
		fmt.Fprintln(os.Stderr, `Usage: mnemos query "<question>"`)
		os.Exit(1)
	}

	query := strings.Join(fs.Args(), " ")

	eng, err := loadMnemosEngine(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer eng.store.Close()

	start := time.Now()
	results, _, _, err := eng.Search(query, *topK)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		resp := struct {
			Query     string                `json:"query"`
			Results   []tui.SearchResult `json:"results"`
			Count     int                   `json:"count"`
			LatencyMs float64               `json:"latency_ms"`
		}{
			Query:     query,
			Results:   results,
			Count:     len(results),
			LatencyMs: float64(elapsed.Milliseconds()),
		}
		json.NewEncoder(os.Stdout).Encode(resp)
	} else {
		fmt.Printf("\nQuery: %s\n", query)
		fmt.Printf("Found %d results in %s\n\n", len(results), elapsed.Round(time.Microsecond))

		for _, r := range results {
			fmt.Printf("  #%d  %s\n", r.Rank, r.Title)
			fmt.Printf("      Path:  %s\n", r.Path)
			fmt.Printf("      Score: %.4f (BM25: %.4f | Embed: %.4f)\n", r.Score, r.BM25Score, r.EmbedScore)
			if r.Snippet != "" {
				snippet := r.Snippet
				if len(snippet) > 200 {
					snippet = snippet[:200] + "..."
				}
				fmt.Printf("      ───\n      %s\n", snippet)
			}
			fmt.Println()
		}
	}
}


func cmdTui(args []string) {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	dataDir := fs.String("data-dir", defaultDataDir, "Data directory")
	fs.Parse(args)

	eng, err := loadMnemosEngine(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer eng.store.Close()

	tui.Run(eng)
}

func cmdStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dataDir := fs.String("data-dir", defaultDataDir, "Data directory")
	fs.Parse(args)

	eng, err := loadMnemosEngine(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer eng.store.Close()

	stats := eng.GetStats()

	fmt.Println("\nMnemos — Engine Statistics")
	fmt.Println("─────────────────────────────")
	for k, v := range stats {
		fmt.Printf("  %-25s %v\n", k+":", v)
	}
	fmt.Println()
}

func cmdCompact(args []string) {
	fs := flag.NewFlagSet("compact", flag.ExitOnError)
	dataDir := fs.String("data-dir", defaultDataDir, "Data directory")
	fs.Parse(args)

	engine, err := initEngine(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	fmt.Fprintln(os.Stderr, "Mnemos: Triggering compaction...")
	if err := engine.Compact(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "  ✓ Compaction complete")
}

// --- Engine initialization and loading ---

func initEngine(dataDir string) (*storage.Engine, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	config := storage.DefaultConfig(absDir)
	return storage.NewEngine(config)
}

func loadExistingHashes(engine *storage.Engine) map[string]bool {
	hashes := make(map[string]bool)
	keys, err := engine.AllKeys()
	if err != nil {
		return hashes
	}
	for _, k := range keys {
		if strings.HasPrefix(k, "hash:") {
			hashes[strings.TrimPrefix(k, "hash:")] = true
		}
	}
	return hashes
}

func loadMnemosEngine(dataDir string) (*MnemosEngine, error) {
	store, err := initEngine(dataDir)
	if err != nil {
		return nil, fmt.Errorf("init storage: %w", err)
	}

	absDir, _ := filepath.Abs(dataDir)

	// Load tokenizer
	tokPath := filepath.Join(absDir, "tokenizer.json")
	tok, err := tokenizer.LoadBPE(tokPath)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	// Load all documents
	docs := make(map[string]ingest.Document)
	docTokens := make(map[string][]string)
	docEmbeddings := make(map[string][]int8)

	keys, err := store.AllKeys()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("list keys: %w", err)
	}

	for _, k := range keys {
		if strings.HasPrefix(k, "doc:") {
			docID := strings.TrimPrefix(k, "doc:")
			data, found, err := store.Get([]byte(k))
			if err != nil || !found {
				continue
			}
			var doc ingest.Document
			if err := json.Unmarshal(data, &doc); err != nil {
				continue
			}
			docs[docID] = doc
		}
	}

	// Load tokens
	for _, k := range keys {
		if strings.HasPrefix(k, "tokens:") {
			docID := strings.TrimPrefix(k, "tokens:")
			data, found, err := store.Get([]byte(k))
			if err != nil || !found {
				continue
			}
			var tokens []string
			if err := json.Unmarshal(data, &tokens); err != nil {
				continue
			}
			docTokens[docID] = tokens
		}
	}

	// Load embeddings
	for _, k := range keys {
		if strings.HasPrefix(k, "embed:") {
			docID := strings.TrimPrefix(k, "embed:")
			data, found, err := store.Get([]byte(k))
			if err != nil || !found {
				continue
			}
			var emb []int8
			if err := json.Unmarshal(data, &emb); err != nil {
				continue
			}
			docEmbeddings[docID] = emb
		}
	}

	// Rebuild BM25 index
	bm25Idx := rank.NewBM25Index()
	for docID, tokens := range docTokens {
		bm25Idx.AddDocument(docID, tokens)
	}

	// Rebuild LSH index
	dims := 100
	if len(docEmbeddings) > 0 {
		for _, emb := range docEmbeddings {
			dims = len(emb)
			break
		}
	}
	lshIdx := index.NewLSHIndex(index.DefaultLSHConfig(), dims)
	for docID, emb := range docEmbeddings {
		lshIdx.Add(docID, emb)
	}

	// Load embedding engine
	embedPath := filepath.Join(absDir, "embeddings.json")
	embedEng, err := embed.LoadEmbeddingEngine(embedPath)
	if err != nil {
		// Fallback to empty engine if missing (e.g. fresh start)
		embedEng = embed.NewEmbeddingEngine(embed.DefaultEmbedConfig())
	}

	// Load MLP ranker
	mlpPath := filepath.Join(absDir, "ranker.mlp")
	mlp, _ := rank.LoadMLP(mlpPath)

	return &MnemosEngine{
		store:         store,
		tok:           tok,
		embedEng:      embedEng,
		lshIndex:      lshIdx,
		bm25Index:     bm25Idx,
		mlp:           mlp,
		docs:          docs,
		docTokens:     docTokens,
		docEmbeddings: docEmbeddings,
		dataDir:       absDir,
	}, nil
}

// Search implements tui.Mnemos interface.
func (e *MnemosEngine) Search(query string, k int) ([]tui.SearchResult, [2]float64, []tui.DataPoint, error) {
	// Tokenize the query
	queryTokens := e.tok.EncodeToTokens(query)

	// BM25 ranking
	bm25Results := e.bm25Index.Query(queryTokens, k*2) // get extra for fusion

	// Embedding-based ranking
	queryEmbed := e.embedEng.DocumentEmbedding(queryTokens)
	quantizedQuery := embed.Quantize(queryEmbed)
	var embedResults []rank.RankedResult
	
	var queryPoint [2]float64
	if len(queryEmbed) >= 2 {
		queryPoint[0] = queryEmbed[0]
		queryPoint[1] = queryEmbed[1]
	}

	if quantizedQuery != nil && e.lshIndex.Size() > 0 {
		lshResults := e.lshIndex.Query(quantizedQuery, k*2)
		for _, r := range lshResults {
			embedResults = append(embedResults, rank.RankedResult{
				DocID: r.DocID,
				Score: r.Score,
			})
		}
	}

	// Reciprocal Rank Fusion / MLP Fusion
	var fusedResults []rank.RankedResult
	if e.mlp != nil {
		fusedResults = rank.FuseRankingsMLP(bm25Results, embedResults, e.mlp, k)
	} else {
		fusedResults = rank.FuseRankings(bm25Results, embedResults, k)
	}

	// Build response with snippets
	results := make([]tui.SearchResult, 0, len(fusedResults))
	for _, r := range fusedResults {
		doc, ok := e.docs[r.DocID]
		if !ok {
			continue
		}

		// Generate TextRank snippet
		snippet := summarize.SummarizeToString(doc.Content, summarize.DefaultTextRankConfig())

		var x, y float64
		if emb, ok := e.docEmbeddings[r.DocID]; ok && len(emb) >= 2 {
			deq := embed.Dequantize(emb)
			x, y = deq[0], deq[1]
		}

		results = append(results, tui.SearchResult{
			DocID:      r.DocID,
			Title:      doc.Title,
			Path:       doc.Path,
			Snippet:    snippet,
			Score:      r.Score,
			BM25Score:  r.BM25Score,
			EmbedScore: r.EmbedScore,
			Rank:       r.Rank,
			X:          x,
			Y:          y,
		})
	}

	// Collect all document points for the visualizer
	var allPoints []tui.DataPoint
	for docID, emb := range e.docEmbeddings {
		if len(emb) >= 2 {
			deq := embed.Dequantize(emb)
			allPoints = append(allPoints, tui.DataPoint{
				ID: docID,
				X:  deq[0],
				Y:  deq[1],
			})
		}
	}

	return results, queryPoint, allPoints, nil
}

// GetStats implements tui.Mnemos interface.
func (e *MnemosEngine) GetStats() map[string]interface{} {
	storageStats := e.store.Stats()

	stats := map[string]interface{}{
		"document_count":     len(e.docs),
		"vocabulary_size":    e.tok.VocabSize(),
		"lsh_index_size":     e.lshIndex.Size(),
		"bm25_doc_count":     e.bm25Index.DocCount,
		"data_dir":           e.dataDir,
	}

	for k, v := range storageStats {
		stats["storage_"+k] = v
	}

	return stats
}
