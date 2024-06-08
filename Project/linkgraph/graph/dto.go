package graph

import (
	"github.com/google/uuid"
)

type Link struct {
	ID          uuid.UUID
	URL         string
	RetrievedAt int64
}

type Edge struct {
	ID        uuid.UUID
	Src       uuid.UUID
	Dst       uuid.UUID
	UpdatedAt int64
}
