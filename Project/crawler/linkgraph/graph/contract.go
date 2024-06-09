package graph

import (
	"github.com/google/uuid"
)

type Graph interface {
	UpsertLink(link *Link) error
	FindLink(id uuid.UUID) (*Link, error)
	UpsertEdge(edge *Edge) error
	RemoveStaleEdges(fromID uuid.UUID, updatedBefore int64) error
	Links(fromID, toID uuid.UUID, retrievedBefore int64) (LinkIterator, error)
	Edges(fromId, toID uuid.UUID, updatedBefore int64) (EdgeIterator, error)
}

// LinkIterator is implemented by objects that can iterate the graph links.
type LinkIterator interface {
	Iterator

	// Link Returns The Currently Fetched Link Object
	Link() *Link
}

// EdgeIterator is implemented by objects that can iterate the graph edges.
type EdgeIterator interface {
	Iterator

	// Edge returns the currently fetched edge objects.
	Edge() *Edge
}

type Iterator interface {
	// Next advances the iterator. If no more items are available or an
	// error occurs, calls to Next() return false.
	Next() bool

	// Error returns the last error encountered by iterator.
	Error() error

	// Close releases any resources associated with an iterator.
	Close() error
}
