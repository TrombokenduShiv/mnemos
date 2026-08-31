// Package embed implements distributional word embeddings using PPMI
// (Positive Pointwise Mutual Information) and power-iteration SVD,
// replacing sentence-transformers, gensim, and scikit-learn's TruncatedSVD.
package embed

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
)

// ppmiEntry represents a single non-zero entry in the PPMI matrix.
type ppmiEntry struct {
	row, col int
	value    float64
}

// EmbedConfig holds parameters for the embedding engine.
type EmbedConfig struct {
	Dimensions    int     // embedding dimensionality (default 100)
	WindowSize    int     // co-occurrence context window (default 5)
	MaxIterations int     // power iteration max iterations (default 50)
	Tolerance     float64 // convergence tolerance (default 1e-6)
	MinFrequency  int     // minimum token frequency to include (default 2)
}

// DefaultEmbedConfig returns sensible defaults.
func DefaultEmbedConfig() EmbedConfig {
	return EmbedConfig{
		Dimensions:    100,
		WindowSize:    5,
		MaxIterations: 50,
		Tolerance:     1e-6,
		MinFrequency:  2,
	}
}

// EmbeddingEngine holds the trained word embeddings and provides lookup.
type EmbeddingEngine struct {
	Config     EmbedConfig
	Vocab      map[string]int  // token → index
	InvVocab   []string        // index → token
	Embeddings [][]float64     // [vocabSize][dimensions]
}

// NewEmbeddingEngine creates an untrained embedding engine.
func NewEmbeddingEngine(config EmbedConfig) *EmbeddingEngine {
	return &EmbeddingEngine{
		Config: config,
		Vocab:  make(map[string]int),
	}
}

// Train builds embeddings from tokenized documents.
// Each document is a slice of string tokens.
func (e *EmbeddingEngine) Train(documents [][]string) {
	// Step 1: Build vocabulary with frequency filtering
	freq := make(map[string]int)
	for _, doc := range documents {
		for _, token := range doc {
			freq[token]++
		}
	}

	// Filter by minimum frequency and build vocab
	e.Vocab = make(map[string]int)
	e.InvVocab = make([]string, 0)
	for token, count := range freq {
		if count >= e.Config.MinFrequency {
			idx := len(e.InvVocab)
			e.Vocab[token] = idx
			e.InvVocab = append(e.InvVocab, token)
		}
	}

	vocabSize := len(e.InvVocab)
	if vocabSize == 0 {
		return
	}

	// Cap vocabulary for computational tractability
	maxVocab := 10000
	if vocabSize > maxVocab {
		// Keep the most frequent tokens
		type tokenFreq struct {
			token string
			freq  int
		}
		sorted := make([]tokenFreq, 0, vocabSize)
		for _, t := range e.InvVocab {
			sorted = append(sorted, tokenFreq{t, freq[t]})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].freq > sorted[j].freq
		})

		e.Vocab = make(map[string]int)
		e.InvVocab = make([]string, maxVocab)
		for i := 0; i < maxVocab; i++ {
			e.Vocab[sorted[i].token] = i
			e.InvVocab[i] = sorted[i].token
		}
		vocabSize = maxVocab
	}

	// Step 2: Build co-occurrence matrix (sparse)
	cooccur := make(map[[2]int]float64)
	totalCooccur := 0.0
	rowSum := make([]float64, vocabSize)
	colSum := make([]float64, vocabSize)

	for _, doc := range documents {
		for i, token := range doc {
			idx1, ok1 := e.Vocab[token]
			if !ok1 {
				continue
			}

			// Context window
			start := i - e.Config.WindowSize
			if start < 0 {
				start = 0
			}
			end := i + e.Config.WindowSize + 1
			if end > len(doc) {
				end = len(doc)
			}

			for j := start; j < end; j++ {
				if j == i {
					continue
				}
				idx2, ok2 := e.Vocab[doc[j]]
				if !ok2 {
					continue
				}

				// Distance-weighted co-occurrence
				dist := math.Abs(float64(i - j))
				weight := 1.0 / dist

				pair := [2]int{idx1, idx2}
				cooccur[pair] += weight
				rowSum[idx1] += weight
				colSum[idx2] += weight
				totalCooccur += weight
			}
		}
	}

	// Step 3: Compute PPMI matrix (sparse)
	var ppmiEntries []ppmiEntry

	for pair, count := range cooccur {
		i, j := pair[0], pair[1]
		pmi := math.Log2((count * totalCooccur) / (rowSum[i] * colSum[j] + 1e-10))
		if pmi > 0 {
			ppmiEntries = append(ppmiEntries, ppmiEntry{i, j, pmi})
		}
	}

	// Step 4: Power-iteration SVD to get top-k singular vectors
	dims := e.Config.Dimensions
	if dims > vocabSize {
		dims = vocabSize
	}

	e.Embeddings = e.powerIterationSVD(ppmiEntries, vocabSize, dims)
}

// powerIterationSVD approximates the top-k singular vectors using the
// power iteration method. This is pure linear algebra with nothing but math
// and nested loops — no matrix library.
func (e *EmbeddingEngine) powerIterationSVD(entries []ppmiEntry, n, k int) [][]float64 {
	rng := rand.New(rand.NewSource(42)) // seeded for reproducibility

	embeddings := make([][]float64, n)
	for i := range embeddings {
		embeddings[i] = make([]float64, k)
	}

	// Build sparse row representation for efficient matrix-vector products
	type sparseEntry struct {
		col   int
		value float64
	}
	rows := make([][]sparseEntry, n)
	for _, e := range entries {
		rows[e.row] = append(rows[e.row], sparseEntry{e.col, e.value})
	}

	// For each singular vector dimension
	for dim := 0; dim < k; dim++ {
		// Initialize random vector
		v := make([]float64, n)
		for i := range v {
			v[i] = rng.NormFloat64()
		}
		normalizeVec(v)

		// Power iteration: v ← normalize(M^T M v)
		for iter := 0; iter < e.Config.MaxIterations; iter++ {
			// Compute M * v
			mv := make([]float64, n)
			for i := 0; i < n; i++ {
				for _, entry := range rows[i] {
					mv[i] += entry.value * v[entry.col]
				}
			}

			// Compute M^T * (M * v)
			mtmv := make([]float64, n)
			for i := 0; i < n; i++ {
				for _, entry := range rows[i] {
					mtmv[entry.col] += entry.value * mv[i]
				}
			}

			// Deflate: remove components of previously found vectors
			for prevDim := 0; prevDim < dim; prevDim++ {
				prev := make([]float64, n)
				for i := 0; i < n; i++ {
					prev[i] = embeddings[i][prevDim]
				}
				proj := dotProduct(mtmv, prev)
				for i := 0; i < n; i++ {
					mtmv[i] -= proj * prev[i]
				}
			}

			// Check convergence
			normalizeVec(mtmv)
			diff := 0.0
			for i := 0; i < n; i++ {
				d := mtmv[i] - v[i]
				diff += d * d
			}

			copy(v, mtmv)

			if math.Sqrt(diff) < e.Config.Tolerance {
				break
			}
		}

		// Store this dimension in the embeddings
		for i := 0; i < n; i++ {
			embeddings[i][dim] = v[i]
		}
	}

	return embeddings
}

// GetEmbedding returns the embedding vector for a token.
func (e *EmbeddingEngine) GetEmbedding(token string) ([]float64, bool) {
	idx, ok := e.Vocab[token]
	if !ok {
		return nil, false
	}
	result := make([]float64, len(e.Embeddings[idx]))
	copy(result, e.Embeddings[idx])
	return result, true
}

// DocumentEmbedding computes a document embedding as the length-normalized
// sum of its constituent token embeddings.
func (e *EmbeddingEngine) DocumentEmbedding(tokens []string) []float64 {
	if len(e.Embeddings) == 0 {
		return nil
	}

	dims := len(e.Embeddings[0])
	sum := make([]float64, dims)
	count := 0

	for _, token := range tokens {
		idx, ok := e.Vocab[token]
		if !ok {
			continue
		}
		for d := 0; d < dims; d++ {
			sum[d] += e.Embeddings[idx][d]
		}
		count++
	}

	if count == 0 {
		return sum
	}

	// Length-normalize
	normalizeVec(sum)
	return sum
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	dot := dotProduct(a, b)
	normA := vecNorm(a)
	normB := vecNorm(b)

	if normA < 1e-10 || normB < 1e-10 {
		return 0
	}

	return dot / (normA * normB)
}

// --- Quantization (INT8) ---

// Quantize compresses a length-normalized float64 vector into an int8 vector.
// This reduces memory and storage footprint by 87.5% (8 bytes to 1 byte per dimension).
func Quantize(v []float64) []int8 {
	if len(v) == 0 {
		return nil
	}
	// Since vectors are typically length normalized, values are in [-1, 1]
	q := make([]int8, len(v))
	for i, val := range v {
		// Clamp to [-1, 1] just in case
		if val > 1.0 { val = 1.0 }
		if val < -1.0 { val = -1.0 }
		q[i] = int8(math.Round(val * 127.0))
	}
	return q
}

// Dequantize inflates an int8 vector back to float64.
func Dequantize(v []int8) []float64 {
	if len(v) == 0 {
		return nil
	}
	f := make([]float64, len(v))
	for i, val := range v {
		f[i] = float64(val) / 127.0
	}
	return f
}

// QuantizedCosineSimilarity computes the cosine similarity directly on int8 vectors.
func QuantizedCosineSimilarity(a, b []int8) float64 {
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

	if normA < 1e-10 || normB < 1e-10 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// --- Linear algebra utilities (pure math, no library) ---

func dotProduct(a, b []float64) float64 {
	sum := 0.0
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func vecNorm(v []float64) float64 {
	return math.Sqrt(dotProduct(v, v))
}

func normalizeVec(v []float64) {
	norm := vecNorm(v)
	if norm < 1e-10 {
		return
	}
	for i := range v {
		v[i] /= norm
	}
}

// Save serializes the EmbeddingEngine to a file.
func (e *EmbeddingEngine) Save(path string) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("embed: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("embed: save: %w", err)
	}
	return nil
}

// LoadEmbeddingEngine deserializes an EmbeddingEngine from a file.
func LoadEmbeddingEngine(path string) (*EmbeddingEngine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("embed: read: %w", err)
	}
	var e EmbeddingEngine
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("embed: unmarshal: %w", err)
	}
	return &e, nil
}
