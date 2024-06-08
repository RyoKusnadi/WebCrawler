package db

import (
	"database/sql"
	"webcrawler/linkgraph/graph"

	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	upsertLinkQuery = `
INSERT INTO links (url, retrieved_at) VALUES ($1, $2) 
ON CONFLICT (url) DO UPDATE SET retrieved_at=GREATEST(links.retrieved_at, $2)
RETURNING id, retrieved_at
`
	findLinkQuery         = "SELECT url, retrieved_at FROM links WHERE id=$1"
	linksInPartitionQuery = "SELECT id, url, retrieved_at FROM links WHERE id >= $1 AND id < $2 AND retrieved_at < $3"

	upsertEdgeQuery = `
INSERT INTO edges (src, dst, updated_at) VALUES ($1, $2, NOW())
ON CONFLICT (src,dst) DO UPDATE SET updated_at=NOW()
RETURNING id, updated_at
`
	edgesInPartitionQuery = "SELECT id, src, dst, updated_at FROM edges WHERE src >= $1 AND src < $2 AND updated_at < $3"
	removeStaleEdgesQuery = "DELETE FROM edges WHERE src=$1 AND updated_at < $2"

	// Compile-time check for ensuring DBGraph implements Graph.
	_ graph.Graph = (*DBGraph)(nil)
)

// DBGraph implements a graph that persists its links and edges to a
// db instance.
type DBGraph struct {
	db *sql.DB
}

// NewDBGraph returns a DBGraph instance that connects to the db
// instance specified by dsn.
func NewDBGraph(dsn string) (*DBGraph, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	return &DBGraph{db: db}, nil
}

// Close terminates the connection to the backing db instance.
func (c *DBGraph) Close() error {
	return c.db.Close()
}

// UpsertLink creates a new link or updates an existing link.
func (c *DBGraph) UpsertLink(link *graph.Link) error {
	row := c.db.QueryRow(upsertLinkQuery, link.URL, link.RetrievedAt)
	if err := row.Scan(&link.ID, &link.RetrievedAt); err != nil {
		return fmt.Errorf("upsert link: %w", err)
	}

	link.RetrievedAt = link.RetrievedAt
	return nil
}

// FindLink looks up a link by its ID.
func (c *DBGraph) FindLink(id uuid.UUID) (*graph.Link, error) {
	row := c.db.QueryRow(findLinkQuery, id)
	link := &graph.Link{ID: id}
	if err := row.Scan(&link.URL, &link.RetrievedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("find link: %w", graph.ErrNotFound)
		}

		return nil, fmt.Errorf("find link: %w", err)
	}

	link.RetrievedAt = link.RetrievedAt
	return link, nil
}

// Links returns an iterator for the set of links whose IDs belong to the
// [fromID, toID) range and were last accessed before the provided value.
func (c *DBGraph) Links(fromID, toID uuid.UUID, accessedBefore int64) (graph.LinkIterator, error) {
	rows, err := c.db.Query(linksInPartitionQuery, fromID, toID, accessedBefore)
	if err != nil {
		return nil, fmt.Errorf("links: %w", err)
	}

	return &linkIterator{rows: rows}, nil
}

// UpsertEdge creates a new edge or updates an existing edge.
func (c *DBGraph) UpsertEdge(edge *graph.Edge) error {
	row := c.db.QueryRow(upsertEdgeQuery, edge.Src, edge.Dst)
	if err := row.Scan(&edge.ID, &edge.UpdatedAt); err != nil {
		if isForeignKeyViolationError(err) {
			err = graph.ErrUnknownEdgeLinks
		}
		return fmt.Errorf("upsert edge: %w", err)
	}

	edge.UpdatedAt = edge.UpdatedAt
	return nil
}

// Edges returns an iterator for the set of edges whose source vertex IDs
// belong to the [fromID, toID) range and were last updated before the provided
// value.
func (c *DBGraph) Edges(fromID, toID uuid.UUID, updatedBefore int64) (graph.EdgeIterator, error) {
	rows, err := c.db.Query(edgesInPartitionQuery, fromID, toID, updatedBefore)
	if err != nil {
		return nil, fmt.Errorf("edges: %w", err)
	}

	return &edgeIterator{rows: rows}, nil
}

// RemoveStaleEdges removes any edge that originates from the specified link ID
// and was updated before the specified timestamp.
func (c *DBGraph) RemoveStaleEdges(fromID uuid.UUID, updatedBefore int64) error {
	_, err := c.db.Exec(removeStaleEdgesQuery, fromID, updatedBefore)
	if err != nil {
		return fmt.Errorf("remove stale edges: %w", err)
	}

	return nil
}

// isForeignKeyViolationError returns true if err indicates a foreign key
// constraint violation.
func isForeignKeyViolationError(err error) bool {
	pqErr, valid := err.(*pq.Error)
	if !valid {
		return false
	}

	return pqErr.Code.Name() == "foreign_key_violation"
}
