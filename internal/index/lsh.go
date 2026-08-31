// Package index implements SimHash Locality-Sensitive Hashing for
// approximate nearest-neighbor search, replacing FAISS and Annoy.
package index

import (
	"math"
	"math/rand"
	"sort"
)

// LSHConfig holds parameters for the SimHash LSH index.
type LSHConfig struct {
	NumHyperplanes int // number of random hyperplanes (hash bits)
	BucketPrefixLen int // how many bits of the hash to use for bucketing
}

// DefaultLSHConfig returns sensible defaults.
func DefaultLSHConfig() LSHConfig {
	return LSHConfig{
		NumHyperplanes:  128,
		BucketPrefixLen: 8,
	}
}

// LSHIndex is a SimHash-based locality-sensitive hashing index for
// approximate nearest-neighbor search over document embeddings.
type LSHIndex struct {
	Config      LSHConfig
	Hyperplanes [][]float64         // [numHyperplanes][dimensions]
	Buckets     map[uint64][]int    // bucket hash → list of document indices
	Signatures  []uint64            // per-document SimHash signatures (first 64 bits)
	FullSigs    [][]bool            // per-document full boolean signatures
	Embeddings  [][]int8            // stored quantized document embeddings
	DocIDs      []string            // document ID for each index position
}

// NewLSHIndex creates a new SimHash LSH index with random hyperplanes.
func NewLSHIndex(config LSHConfig, dimensions int) *LSHIndex {
	rng := rand.New(rand.NewSource(42)) // seeded for reproducibility

	// Generate random hyperplanes
	hyperplanes := make([][]float64, config.NumHyperplanes)
	for i := range hyperplanes {
		hp := make([]float64, dimensions)
		for j := range hp {
			hp[j] = rng.NormFloat64()
		}
		// Normalize
		norm := 0.0
		for _, v := range hp {
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm > 1e-10 {
			for j := range hp {
				hp[j] /= norm
			}
		}
		hyperplanes[i] = hp
	}

	return &LSHIndex{
		Config:      config,
		Hyperplanes: hyperplanes,
		Buckets:     make(map[uint64][]int),
		Signatures:  make([]uint64, 0),
		FullSigs:    make([][]bool, 0),
		Embeddings:  make([][]int8, 0),
		DocIDs:      make([]string, 0),
	}
}

// Add indexes a document embedding with its ID.
func (idx *LSHIndex) Add(docID string, embedding []int8) {
	pos := len(idx.Embeddings)

	// Compute SimHash signature
	sig := idx.computeSignature(embedding)
	fullSig := idx.computeFullSignature(embedding)

	// Compute bucket key from prefix bits
	bucketKey := idx.bucketKey(sig)

	// Store
	idx.Embeddings = append(idx.Embeddings, embedding)
	idx.DocIDs = append(idx.DocIDs, docID)
	idx.Signatures = append(idx.Signatures, sig)
	idx.FullSigs = append(idx.FullSigs, fullSig)
	idx.Buckets[bucketKey] = append(idx.Buckets[bucketKey], pos)
}

// Query finds the k most similar documents to the query embedding.
// Uses LSH for candidate selection, then exact cosine similarity for ranking.
func (idx *LSHIndex) Query(queryEmbedding []int8, k int) []SearchResult {
	if len(idx.Embeddings) == 0 {
		return nil
	}

	// Compute query signature
	querySig := idx.computeSignature(queryEmbedding)
	queryBucket := idx.bucketKey(querySig)

	// Collect candidates from the query's bucket and neighboring buckets
	candidateSet := make(map[int]bool)

	// Primary bucket
	for _, pos := range idx.Buckets[queryBucket] {
		candidateSet[pos] = true
	}

	// If not enough candidates, try neighboring buckets (flip bits in the prefix)
	if len(candidateSet) < k*2 {
		for bit := 0; bit < idx.Config.BucketPrefixLen && bit < 64; bit++ {
			neighborKey := queryBucket ^ (1 << uint(bit))
			for _, pos := range idx.Buckets[neighborKey] {
				candidateSet[pos] = true
			}
		}
	}

	// If still not enough, fall back to brute force
	if len(candidateSet) < k {
		for i := range idx.Embeddings {
			candidateSet[i] = true
		}
	}

	// Compute exact cosine similarity for all candidates
	results := make([]SearchResult, 0, len(candidateSet))
	for pos := range candidateSet {
		sim := cosineSim(queryEmbedding, idx.Embeddings[pos])
		results = append(results, SearchResult{
			DocID:      idx.DocIDs[pos],
			Score:      sim,
			Position:   pos,
		})
	}

	// Sort by similarity descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top k
	if len(results) > k {
		results = results[:k]
	}

	return results
}

// BruteForceQuery performs an exact nearest-neighbor search (for benchmarking).
func (idx *LSHIndex) BruteForceQuery(queryEmbedding []int8, k int) []SearchResult {
	results := make([]SearchResult, 0, len(idx.Embeddings))
	for i, emb := range idx.Embeddings {
		sim := cosineSim(queryEmbedding, emb)
		results = append(results, SearchResult{
			DocID:    idx.DocIDs[i],
			Score:    sim,
			Position: i,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > k {
		results = results[:k]
	}

	return results
}

// SearchResult holds a single search result.
type SearchResult struct {
	DocID    string
	Score    float64
	Position int
}

// computeSignature computes a 64-bit SimHash signature.
func (idx *LSHIndex) computeSignature(embedding []int8) uint64 {
	var sig uint64
	bits := 64
	if idx.Config.NumHyperplanes < bits {
		bits = idx.Config.NumHyperplanes
	}

	for i := 0; i < bits; i++ {
		dot := 0.0
		for j := 0; j < len(embedding) && j < len(idx.Hyperplanes[i]); j++ {
			dot += float64(embedding[j]) * idx.Hyperplanes[i][j]
		}
		if dot >= 0 {
			sig |= (1 << uint(i))
		}
	}
	return sig
}

// computeFullSignature computes the full boolean signature.
func (idx *LSHIndex) computeFullSignature(embedding []int8) []bool {
	sig := make([]bool, idx.Config.NumHyperplanes)
	for i := 0; i < idx.Config.NumHyperplanes; i++ {
		dot := 0.0
		for j := 0; j < len(embedding) && j < len(idx.Hyperplanes[i]); j++ {
			dot += float64(embedding[j]) * idx.Hyperplanes[i][j]
		}
		sig[i] = dot >= 0
	}
	return sig
}

// bucketKey extracts the bucket key from a signature.
func (idx *LSHIndex) bucketKey(sig uint64) uint64 {
	mask := uint64((1 << uint(idx.Config.BucketPrefixLen)) - 1)
	return sig & mask
}

// Size returns the number of indexed documents.
func (idx *LSHIndex) Size() int {
	return len(idx.Embeddings)
}

// HammingDistance computes the Hamming distance between two signatures.
func HammingDistance(a, b []bool) int {
	dist := 0
	for i := range a {
		if i < len(b) && a[i] != b[i] {
			dist++
		}
	}
	return dist
}

func cosineSim(a, b []int8) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		va := float64(a[i])
		vb := float64(b[i])
		dot += va * vb
		normA += va * va
		normB += vb * vb
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom < 1e-10 {
		return 0
	}
	return dot / denom
}
