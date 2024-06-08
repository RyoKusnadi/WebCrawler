package memory

import (
	"sync"
	"webcrawler/textindexer/index"

	"github.com/blevesearch/bleve/v2"
)

type bleveDoc struct {
	Title    string
	Content  string
	PageRank float64
}

// InMemoryBleveIndexer is an Indexer implementation that uses an in-memory
// bleve instance to catalogue and search documents.
type InMemoryBleveIndexer struct {
	mu   sync.RWMutex
	docs map[string]*index.Document

	idx bleve.Index
}
