package memory

import (
	"fmt"
	"time"
	"webcrawler/crawler/linkgraph/graph"

	"github.com/google/uuid"
)

// Compile-time check for ensuring InMemoryGraph implements Graph.
var _ graph.Graph = (*InMemoryGraph)(nil)

// NewInMemoryGraph creates a new in-memory link graph.
func NewInMemoryGraph() *InMemoryGraph {
	return &InMemoryGraph{
		links:        make(map[uuid.UUID]*graph.Link),
		edges:        make(map[uuid.UUID]*graph.Edge),
		linkURLIndex: make(map[string]*graph.Link),
		linkEdgeMap:  make(map[uuid.UUID]edgeList),
	}
}

// UpsertLink creates a new link or updates an existing link.
func (s *InMemoryGraph) UpsertLink(link *graph.Link) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if a link with the same URL already exists. If so, convert
	// this into an update and point the link ID to the existing link.
	if existing := s.linkURLIndex[link.URL]; existing != nil {
		link.ID = existing.ID
		origTs := existing.RetrievedAt
		*existing = *link
		if origTs > existing.RetrievedAt {
			existing.RetrievedAt = origTs
		}
		return nil
	}

	// Assign new ID and insert link
	for {
		link.ID = uuid.New()
		if s.links[link.ID] == nil {
			break
		}
	}

	lCopy := new(graph.Link)
	*lCopy = *link
	s.linkURLIndex[lCopy.URL] = lCopy
	s.links[lCopy.ID] = lCopy
	return nil
}

// FindLink looks up a link by its ID.
func (s *InMemoryGraph) FindLink(id uuid.UUID) (*graph.Link, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	link := s.links[id]
	if link == nil {
		return nil, fmt.Errorf("find link: %w", graph.ErrNotFound)
	}

	lCopy := new(graph.Link)
	*lCopy = *link
	return lCopy, nil
}

// Links returns an iterator for the set of links whose IDs belong to the
// [fromID, toID) range and were retrieved before the provided unix timestamp.
func (s *InMemoryGraph) Links(fromID, toID uuid.UUID, retrievedBefore int64) (graph.LinkIterator, error) {
	from, to := fromID.String(), toID.String()

	s.mu.RLock()
	var list []*graph.Link
	for linkID, link := range s.links {
		if id := linkID.String(); id >= from && id < to && link.RetrievedAt < retrievedBefore {
			list = append(list, link)
		}
	}
	s.mu.RUnlock()

	return &linkIterator{s: s, links: list}, nil
}

// UpsertEdge creates a new edge or updates an existing edge.
func (s *InMemoryGraph) UpsertEdge(edge *graph.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, srcExists := s.links[edge.Src]
	_, dstExists := s.links[edge.Dst]
	if !srcExists || !dstExists {
		return fmt.Errorf("upsert edge: %w", graph.ErrUnknownEdgeLinks)
	}

	// Scan edge list from source
	for _, edgeID := range s.linkEdgeMap[edge.Src] {
		existingEdge := s.edges[edgeID]
		if existingEdge.Src == edge.Src && existingEdge.Dst == edge.Dst {
			existingEdge.UpdatedAt = time.Now().Unix()
			*edge = *existingEdge
			return nil
		}
	}

	// Insert new edge
	for {
		edge.ID = uuid.New()
		if s.edges[edge.ID] == nil {
			break
		}
	}

	edge.UpdatedAt = time.Now().Unix()
	eCopy := new(graph.Edge)
	*eCopy = *edge
	s.edges[eCopy.ID] = eCopy

	// Append the edge ID to the list of edges originating from the
	// edge's source link.
	s.linkEdgeMap[edge.Src] = append(s.linkEdgeMap[edge.Src], eCopy.ID)
	return nil
}

// Edges returns an iterator for the set of edges whose source vertex IDs
// belong to the [fromID, toID) range and were updated before the provided
// unix timestamp.
func (s *InMemoryGraph) Edges(fromID, toID uuid.UUID, updatedBefore int64) (graph.EdgeIterator, error) {
	from, to := fromID.String(), toID.String()

	s.mu.RLock()
	var list []*graph.Edge
	for linkID := range s.links {
		if id := linkID.String(); id < from || id >= to {
			continue
		}

		for _, edgeID := range s.linkEdgeMap[linkID] {
			if edge := s.edges[edgeID]; edge.UpdatedAt < updatedBefore {
				list = append(list, edge)
			}
		}
	}
	s.mu.RUnlock()

	return &edgeIterator{s: s, edges: list}, nil
}

// RemoveStaleEdges removes any edge that originates from the specified link ID
// and was updated before the specified timestamp.
func (s *InMemoryGraph) RemoveStaleEdges(fromID uuid.UUID, updatedBefore int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newEdgeList edgeList
	for _, edgeID := range s.linkEdgeMap[fromID] {
		edge := s.edges[edgeID]
		if edge.UpdatedAt < updatedBefore {
			delete(s.edges, edgeID)
			continue
		}

		newEdgeList = append(newEdgeList, edgeID)
	}

	// Replace edge list or origin link with the filtered edge list
	s.linkEdgeMap[fromID] = newEdgeList
	return nil
}
