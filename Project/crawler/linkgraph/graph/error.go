package graph

import "errors"

var (
	ErrNotFound = errors.New("not found")

	// ErrUnknownEdgeLinks is returned when attempting to create an edge
	// with an invalid source and/or destination ID
	ErrUnknownEdgeLinks = errors.New("unknown source and/or destination for edge")
)
