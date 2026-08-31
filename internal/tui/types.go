package tui

// DataPoint represents a 2D coordinate in the embedding space for visualization.
type DataPoint struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

// SearchResult represents a single search result from the engine.
type SearchResult struct {
	DocID      string  `json:"doc_id"`
	Title      string  `json:"title"`
	Path       string  `json:"path"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	BM25Score  float64 `json:"bm25_score"`
	EmbedScore float64 `json:"embed_score"`
	Rank       int     `json:"rank"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}
