// Package rank implements BM25 keyword ranking and Reciprocal Rank Fusion (RRF)
// for combining heterogeneous rankers, replacing rank_bm25 (PyPI).
package rank

import (
	"math"
	"sort"
)

// BM25 parameters
const (
	DefaultK1 = 1.5
	DefaultB  = 0.75
)

// BM25Index is an inverted index with BM25 scoring capability.
type BM25Index struct {
	K1          float64
	B           float64
	DocCount    int
	AvgDocLen   float64
	DocLengths  map[string]int                // docID → document length in tokens
	InvertedIdx map[string]map[string]int     // token → { docID → term frequency }
	DocFreq     map[string]int                // token → number of docs containing it
}

// NewBM25Index creates a new BM25 index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		K1:          DefaultK1,
		B:           DefaultB,
		DocLengths:  make(map[string]int),
		InvertedIdx: make(map[string]map[string]int),
		DocFreq:     make(map[string]int),
	}
}

// AddDocument indexes a tokenized document for BM25 search.
func (idx *BM25Index) AddDocument(docID string, tokens []string) {
	idx.DocCount++
	idx.DocLengths[docID] = len(tokens)

	// Update average doc length
	totalLen := 0
	for _, l := range idx.DocLengths {
		totalLen += l
	}
	idx.AvgDocLen = float64(totalLen) / float64(idx.DocCount)

	// Count term frequencies for this document
	seen := make(map[string]bool)
	for _, token := range tokens {
		if _, ok := idx.InvertedIdx[token]; !ok {
			idx.InvertedIdx[token] = make(map[string]int)
		}
		idx.InvertedIdx[token][docID]++

		if !seen[token] {
			idx.DocFreq[token]++
			seen[token] = true
		}
	}
}

// Score computes the BM25 score for a document given query tokens.
func (idx *BM25Index) Score(docID string, queryTokens []string) float64 {
	score := 0.0
	docLen := idx.DocLengths[docID]

	for _, qt := range queryTokens {
		df := idx.DocFreq[qt]
		if df == 0 {
			continue
		}

		tf := 0
		if docTerms, ok := idx.InvertedIdx[qt]; ok {
			tf = docTerms[docID]
		}
		if tf == 0 {
			continue
		}

		// IDF component: log((N - df + 0.5) / (df + 0.5) + 1)
		idf := math.Log(float64(idx.DocCount-df)+0.5) - math.Log(float64(df)+0.5) + math.Log(1)
		if idf < 0 {
			idf = 0 // Clamp negative IDF
		}

		// TF component
		tfNorm := (float64(tf) * (idx.K1 + 1)) /
			(float64(tf) + idx.K1*(1-idx.B+idx.B*float64(docLen)/idx.AvgDocLen))

		score += idf * tfNorm
	}

	return score
}

// Query returns the top-k documents ranked by BM25 score for the query tokens.
func (idx *BM25Index) Query(queryTokens []string, k int) []RankedResult {
	// Collect candidate documents (any doc containing at least one query token)
	candidates := make(map[string]bool)
	for _, qt := range queryTokens {
		if docs, ok := idx.InvertedIdx[qt]; ok {
			for docID := range docs {
				candidates[docID] = true
			}
		}
	}

	// Score all candidates
	results := make([]RankedResult, 0, len(candidates))
	for docID := range candidates {
		score := idx.Score(docID, queryTokens)
		if score > 0 {
			results = append(results, RankedResult{
				DocID:    docID,
				Score:    score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// RankedResult holds a single ranking result.
type RankedResult struct {
	DocID     string
	Score     float64
	BM25Score float64
	EmbedScore float64
	Rank      int
}

// --- Reciprocal Rank Fusion (RRF) ---

const rrfK = 60 // standard RRF constant

// FuseRankings combines two ranked lists using Reciprocal Rank Fusion.
// bm25Results and embedResults are independently ranked lists.
// Returns a fused ranking with all individual scores preserved.
func FuseRankings(bm25Results, embedResults []RankedResult, k int) []RankedResult {
	// Build rank maps
	bm25Rank := make(map[string]int)
	bm25Score := make(map[string]float64)
	for i, r := range bm25Results {
		bm25Rank[r.DocID] = i + 1
		bm25Score[r.DocID] = r.Score
	}

	embedRank := make(map[string]int)
	embedScore := make(map[string]float64)
	for i, r := range embedResults {
		embedRank[r.DocID] = i + 1
		embedScore[r.DocID] = r.Score
	}

	// Collect all unique doc IDs
	allDocs := make(map[string]bool)
	for _, r := range bm25Results {
		allDocs[r.DocID] = true
	}
	for _, r := range embedResults {
		allDocs[r.DocID] = true
	}

	// Compute RRF scores
	results := make([]RankedResult, 0, len(allDocs))
	for docID := range allDocs {
		rrfScore := 0.0

		if rank, ok := bm25Rank[docID]; ok {
			rrfScore += 1.0 / float64(rrfK+rank)
		}
		if rank, ok := embedRank[docID]; ok {
			rrfScore += 1.0 / float64(rrfK+rank)
		}

		results = append(results, RankedResult{
			DocID:      docID,
			Score:      rrfScore,
			BM25Score:  bm25Score[docID],
			EmbedScore: embedScore[docID],
		})
	}

	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Assign final ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// FuseRankingsMLP combines rankings using a trained Multi-Layer Perceptron.
func FuseRankingsMLP(bm25Results, embedResults []RankedResult, mlp *MLP, k int) []RankedResult {
	bm25Score := make(map[string]float64)
	for _, r := range bm25Results {
		bm25Score[r.DocID] = r.Score
	}

	embedScore := make(map[string]float64)
	for _, r := range embedResults {
		embedScore[r.DocID] = r.Score
	}

	allDocs := make(map[string]bool)
	for _, r := range bm25Results {
		allDocs[r.DocID] = true
	}
	for _, r := range embedResults {
		allDocs[r.DocID] = true
	}

	// First compute RRF for the third feature
	rrfResults := FuseRankings(bm25Results, embedResults, len(allDocs))
	rrfScoreMap := make(map[string]float64)
	for _, r := range rrfResults {
		rrfScoreMap[r.DocID] = r.Score
	}

	results := make([]RankedResult, 0, len(allDocs))
	for docID := range allDocs {
		bScore := bm25Score[docID]
		eScore := embedScore[docID]
		rScore := rrfScoreMap[docID]

		// Forward pass
		mlpScore := mlp.Forward([]float64{bScore, eScore, rScore})

		results = append(results, RankedResult{
			DocID:      docID,
			Score:      mlpScore,
			BM25Score:  bScore,
			EmbedScore: eScore,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	for i := range results {
		results[i].Rank = i + 1
	}

	if len(results) > k {
		results = results[:k]
	}

	return results
}

