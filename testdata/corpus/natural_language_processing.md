# Natural Language Processing

Natural Language Processing (NLP) is a field at the intersection of computer science, artificial intelligence, and linguistics, focused on enabling computers to understand, interpret, and generate human language. The field has undergone a dramatic transformation with the advent of deep learning and large language models.

## Tokenization

Tokenization is the process of breaking text into smaller units called tokens. Word-level tokenization splits text at whitespace and punctuation boundaries. Subword tokenization methods like Byte-Pair Encoding (BPE) and WordPiece split words into smaller units, allowing models to handle out-of-vocabulary words by composing them from known subword units. BPE iteratively merges the most frequent pair of adjacent symbols until a desired vocabulary size is reached.

## Word Embeddings

Word embeddings represent words as dense vectors in a continuous vector space where semantically similar words are mapped to nearby points. Early approaches like Word2Vec and GloVe learned embeddings from co-occurrence statistics. The distributional hypothesis — that words appearing in similar contexts have similar meanings — underpins these methods.

Positive Pointwise Mutual Information (PPMI) matrices capture word co-occurrence statistics, and dimensionality reduction techniques like SVD (Singular Value Decomposition) can produce dense embeddings from these sparse matrices. This classical approach, known as Latent Semantic Analysis, predates neural methods but remains theoretically important.

## Information Retrieval

Information retrieval systems help users find relevant documents from large collections. The BM25 algorithm is a probabilistic ranking function widely used in search engines. It considers term frequency, inverse document frequency, and document length to score document relevance for a query.

Modern retrieval systems often combine keyword-based methods like BM25 with semantic search using dense vector embeddings. Reciprocal Rank Fusion (RRF) is a simple but effective technique for combining rankings from multiple retrieval methods into a single fused ranking.

## Extractive Summarization

Extractive summarization selects the most important sentences from a document to create a summary. The TextRank algorithm builds a graph of sentence similarities and uses PageRank-like iteration to identify the most central sentences. Unlike abstractive summarization, extractive methods never generate new text — every word in the summary was written by the original author.

## Semantic Search

Semantic search goes beyond keyword matching to understand the meaning and intent behind a query. By representing both queries and documents as vectors in the same embedding space, semantic search can find relevant documents even when they share no keywords with the query. Approximate nearest neighbor algorithms like SimHash and locality-sensitive hashing make this computationally feasible for large document collections.
