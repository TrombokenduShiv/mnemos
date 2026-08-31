// Package tokenizer implements Byte-Pair Encoding (BPE) from scratch,
// replacing HuggingFace's tokenizers package. Follows Sennrich et al. (2015).
package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// BPE is a Byte-Pair Encoding tokenizer that learns subword units from a corpus.
type BPE struct {
	MergeRules []MergeRule        `json:"merge_rules"`
	Vocab      map[string]int     `json:"vocab"`      // token → ID
	InvVocab   map[int]string     `json:"inv_vocab"`   // ID → token
	NextID     int                `json:"next_id"`
	Config     BPEConfig          `json:"config"`
}

// BPEConfig holds configurable parameters.
type BPEConfig struct {
	NumMerges    int `json:"num_merges"`    // default 8000
	MinFrequency int `json:"min_frequency"` // minimum pair frequency to merge
}

// MergeRule records a single BPE merge operation.
type MergeRule struct {
	Left  string `json:"left"`
	Right string `json:"right"`
	New   string `json:"new"`
}

// DefaultBPEConfig returns sensible defaults.
func DefaultBPEConfig() BPEConfig {
	return BPEConfig{
		NumMerges:    8000,
		MinFrequency: 2,
	}
}

// NewBPE creates a new, untrained BPE tokenizer.
func NewBPE(config BPEConfig) *BPE {
	return &BPE{
		MergeRules: make([]MergeRule, 0, config.NumMerges),
		Vocab:      make(map[string]int),
		InvVocab:   make(map[int]string),
		NextID:     0,
		Config:     config,
	}
}

// preTokenize splits text into words/tokens at whitespace and punctuation boundaries.
func preTokenize(text string) []string {
	var words []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		} else if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			words = append(words, string(r))
		} else {
			current.WriteRune(unicode.ToLower(r))
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

// wordToSymbols splits a word into individual characters/runes for BPE training.
func wordToSymbols(word string) []string {
	symbols := make([]string, 0, utf8.RuneCountInString(word))
	for _, r := range word {
		symbols = append(symbols, string(r))
	}
	return symbols
}

// Train learns BPE merge rules from a corpus of text.
func (b *BPE) Train(texts []string) {
	// Step 1: Build word frequency map
	wordFreq := make(map[string]int)
	for _, text := range texts {
		for _, word := range preTokenize(text) {
			wordFreq[word]++
		}
	}

	// Step 2: Initialize with character-level tokens
	// Build splits: each word → list of characters
	type wordSplit struct {
		symbols []string
		freq    int
	}

	splits := make([]wordSplit, 0, len(wordFreq))
	charSet := make(map[string]bool)

	for word, freq := range wordFreq {
		syms := wordToSymbols(word)
		splits = append(splits, wordSplit{symbols: syms, freq: freq})
		for _, s := range syms {
			charSet[s] = true
		}
	}

	// Initialize vocabulary with individual characters
	for ch := range charSet {
		if _, exists := b.Vocab[ch]; !exists {
			b.Vocab[ch] = b.NextID
			b.InvVocab[b.NextID] = ch
			b.NextID++
		}
	}

	// Step 3: Iteratively merge most frequent pairs
	for mergeNum := 0; mergeNum < b.Config.NumMerges; mergeNum++ {
		// Count all adjacent pairs across all words
		pairFreq := make(map[[2]string]int)
		for _, ws := range splits {
			if len(ws.symbols) < 2 {
				continue
			}
			for i := 0; i < len(ws.symbols)-1; i++ {
				pair := [2]string{ws.symbols[i], ws.symbols[i+1]}
				pairFreq[pair] += ws.freq
			}
		}

		if len(pairFreq) == 0 {
			break
		}

		// Find the most frequent pair
		var bestPair [2]string
		bestFreq := 0
		for pair, freq := range pairFreq {
			if freq > bestFreq || (freq == bestFreq && pair[0]+pair[1] < bestPair[0]+bestPair[1]) {
				bestPair = pair
				bestFreq = freq
			}
		}

		if bestFreq < b.Config.MinFrequency {
			break
		}

		// Create new merged token
		newToken := bestPair[0] + bestPair[1]
		b.MergeRules = append(b.MergeRules, MergeRule{
			Left:  bestPair[0],
			Right: bestPair[1],
			New:   newToken,
		})

		if _, exists := b.Vocab[newToken]; !exists {
			b.Vocab[newToken] = b.NextID
			b.InvVocab[b.NextID] = newToken
			b.NextID++
		}

		// Apply the merge to all word splits
		for idx := range splits {
			splits[idx].symbols = applyMerge(splits[idx].symbols, bestPair[0], bestPair[1], newToken)
		}
	}
}

// applyMerge replaces all occurrences of (left, right) with newToken in symbols.
func applyMerge(symbols []string, left, right, newToken string) []string {
	if len(symbols) < 2 {
		return symbols
	}

	result := make([]string, 0, len(symbols))
	i := 0
	for i < len(symbols) {
		if i < len(symbols)-1 && symbols[i] == left && symbols[i+1] == right {
			result = append(result, newToken)
			i += 2
		} else {
			result = append(result, symbols[i])
			i++
		}
	}
	return result
}

// Encode tokenizes a text string into token IDs using learned merge rules.
func (b *BPE) Encode(text string) []int {
	words := preTokenize(text)
	var ids []int

	for _, word := range words {
		symbols := wordToSymbols(word)

		// Apply merge rules in order
		for _, rule := range b.MergeRules {
			symbols = applyMerge(symbols, rule.Left, rule.Right, rule.New)
		}

		// Convert symbols to IDs
		for _, sym := range symbols {
			if id, ok := b.Vocab[sym]; ok {
				ids = append(ids, id)
			} else {
				// Unknown symbol fallback — split into bytes
				for _, r := range sym {
					s := string(r)
					if id, ok := b.Vocab[s]; ok {
						ids = append(ids, id)
					}
					// If still unknown, skip (graceful degradation)
				}
			}
		}
	}

	return ids
}

// EncodeToTokens tokenizes text into string tokens (useful for BM25 and display).
func (b *BPE) EncodeToTokens(text string) []string {
	words := preTokenize(text)
	var tokens []string

	for _, word := range words {
		symbols := wordToSymbols(word)

		// Apply merge rules in order
		for _, rule := range b.MergeRules {
			symbols = applyMerge(symbols, rule.Left, rule.Right, rule.New)
		}

		tokens = append(tokens, symbols...)
	}

	return tokens
}

// Decode converts token IDs back to text.
func (b *BPE) Decode(ids []int) string {
	var builder strings.Builder
	for i, id := range ids {
		if token, ok := b.InvVocab[id]; ok {
			if i > 0 {
				builder.WriteRune(' ')
			}
			builder.WriteString(token)
		}
	}
	return builder.String()
}

// VocabSize returns the current vocabulary size.
func (b *BPE) VocabSize() int {
	return len(b.Vocab)
}

// Save serializes the tokenizer to a JSON file.
func (b *BPE) Save(path string) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("tokenizer: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadBPE loads a previously trained tokenizer from a JSON file.
func LoadBPE(path string) (*BPE, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tokenizer: read %s: %w", path, err)
	}

	var b BPE
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("tokenizer: unmarshal: %w", err)
	}
	return &b, nil
}

// TokenFrequencies computes term frequencies for a tokenized document.
// Returns a map of token → count.
func TokenFrequencies(tokens []string) map[string]int {
	freq := make(map[string]int, len(tokens))
	for _, t := range tokens {
		freq[t]++
	}
	return freq
}

// TopTokens returns the N most frequent tokens in the vocabulary.
func (b *BPE) TopTokens(n int) []string {
	type tokenCount struct {
		token string
		id    int
	}
	pairs := make([]tokenCount, 0, len(b.Vocab))
	for token, id := range b.Vocab {
		pairs = append(pairs, tokenCount{token, id})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].id < pairs[j].id
	})

	result := make([]string, 0, n)
	for i := 0; i < n && i < len(pairs); i++ {
		result = append(result, pairs[i].token)
	}
	return result
}
