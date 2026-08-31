// Package summarize implements TextRank extractive summarization
// (Mihalcea & Tarau, 2004) to provide concise document snippets.
// Replaces sumy and other summarization libraries.
package summarize

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// TextRankConfig holds parameters for the TextRank summarizer.
type TextRankConfig struct {
	MaxSentences    int     // maximum sentences to return
	MaxCharBudget   int     // maximum total characters in the snippet
	DampingFactor   float64 // PageRank damping factor (default 0.85)
	MaxIterations   int     // power iteration max iterations
	Tolerance       float64 // convergence tolerance
}

// DefaultTextRankConfig returns sensible defaults.
func DefaultTextRankConfig() TextRankConfig {
	return TextRankConfig{
		MaxSentences:  3,
		MaxCharBudget: 500,
		DampingFactor: 0.85,
		MaxIterations: 50,
		Tolerance:     1e-6,
	}
}

// SentenceScore holds a sentence and its TextRank score.
type SentenceScore struct {
	Sentence string
	Score    float64
	Index    int // original position in the document
}

// Summarize extracts the most representative sentences from a document
// using the TextRank algorithm with power iteration.
func Summarize(text string, config TextRankConfig) []SentenceScore {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil
	}

	if len(sentences) <= config.MaxSentences {
		result := make([]SentenceScore, len(sentences))
		for i, s := range sentences {
			result[i] = SentenceScore{Sentence: s, Score: 1.0, Index: i}
		}
		return result
	}

	// Build sentence similarity graph
	n := len(sentences)

	// Tokenize each sentence into a bag of words
	sentTokens := make([]map[string]int, n)
	for i, s := range sentences {
		sentTokens[i] = tokenizeSentence(s)
	}

	// Build adjacency matrix based on cosine similarity of word overlap
	adj := make([][]float64, n)
	for i := range adj {
		adj[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			sim := sentenceSimilarity(sentTokens[i], sentTokens[j])
			adj[i][j] = sim
			adj[j][i] = sim
		}
	}

	// Power iteration to compute PageRank-style centrality scores
	scores := powerIterationRank(adj, n, config.DampingFactor, config.MaxIterations, config.Tolerance)

	// Build scored sentences
	results := make([]SentenceScore, n)
	for i := range sentences {
		results[i] = SentenceScore{
			Sentence: sentences[i],
			Score:    scores[i],
			Index:    i,
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Take top sentences
	maxSent := config.MaxSentences
	if maxSent > len(results) {
		maxSent = len(results)
	}
	top := results[:maxSent]

	// Re-sort by original position for coherent reading order
	sort.Slice(top, func(i, j int) bool {
		return top[i].Index < top[j].Index
	})

	// Enforce character budget
	var finalResults []SentenceScore
	totalChars := 0
	for _, s := range top {
		if totalChars+len(s.Sentence) > config.MaxCharBudget && len(finalResults) > 0 {
			break
		}
		finalResults = append(finalResults, s)
		totalChars += len(s.Sentence)
	}

	return finalResults
}

// SummarizeToString returns the summary as a single string.
func SummarizeToString(text string, config TextRankConfig) string {
	scores := Summarize(text, config)
	if len(scores) == 0 {
		// Fall back to first N characters
		if len(text) > config.MaxCharBudget {
			return text[:config.MaxCharBudget] + "..."
		}
		return text
	}

	parts := make([]string, len(scores))
	for i, s := range scores {
		parts[i] = s.Sentence
	}
	return strings.Join(parts, " ")
}

// powerIterationRank computes PageRank-style centrality using power iteration.
func powerIterationRank(adj [][]float64, n int, damping float64, maxIter int, tol float64) []float64 {
	// Initialize scores uniformly
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = 1.0 / float64(n)
	}

	// Compute out-degree (sum of edge weights) for normalization
	outDeg := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			outDeg[i] += adj[i][j]
		}
	}

	for iter := 0; iter < maxIter; iter++ {
		newScores := make([]float64, n)

		for i := 0; i < n; i++ {
			sum := 0.0
			for j := 0; j < n; j++ {
				if outDeg[j] > 1e-10 {
					sum += adj[j][i] * scores[j] / outDeg[j]
				}
			}
			newScores[i] = (1-damping)/float64(n) + damping*sum
		}

		// Check convergence
		diff := 0.0
		for i := 0; i < n; i++ {
			d := newScores[i] - scores[i]
			diff += d * d
		}

		scores = newScores

		if math.Sqrt(diff) < tol {
			break
		}
	}

	return scores
}

// splitSentences splits text into sentences using rule-based boundary detection.
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		current.WriteRune(runes[i])

		isSentEnd := runes[i] == '.' || runes[i] == '!' || runes[i] == '?'
		if isSentEnd {
			nextIsBreak := i+1 >= len(runes) ||
				unicode.IsSpace(runes[i+1]) ||
				unicode.IsUpper(runes[i+1])

			if nextIsBreak {
				sentence := strings.TrimSpace(current.String())
				if len(sentence) > 15 {
					sentences = append(sentences, sentence)
				}
				current.Reset()
			}
		}

		// Paragraph boundary
		if runes[i] == '\n' && i+1 < len(runes) && runes[i+1] == '\n' {
			sentence := strings.TrimSpace(current.String())
			if len(sentence) > 15 {
				sentences = append(sentences, sentence)
			}
			current.Reset()
		}
	}

	remaining := strings.TrimSpace(current.String())
	if len(remaining) > 15 {
		sentences = append(sentences, remaining)
	}

	return sentences
}

// tokenizeSentence creates a bag-of-words from a sentence.
func tokenizeSentence(sentence string) map[string]int {
	words := make(map[string]int)
	var current strings.Builder

	for _, r := range strings.ToLower(sentence) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 2 {
				words[current.String()]++
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		words[current.String()]++
	}

	return words
}

// sentenceSimilarity computes cosine similarity between two sentence bag-of-words.
func sentenceSimilarity(a, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	// Compute dot product
	dot := 0.0
	for word, countA := range a {
		if countB, ok := b[word]; ok {
			dot += float64(countA) * float64(countB)
		}
	}

	// Compute norms
	normA := 0.0
	for _, count := range a {
		normA += float64(count) * float64(count)
	}
	normB := 0.0
	for _, count := range b {
		normB += float64(count) * float64(count)
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom < 1e-10 {
		return 0
	}

	return dot / denom
}
