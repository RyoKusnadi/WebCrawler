package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"webcrawler/textindexer/index"

	"github.com/elastic/go-elasticsearch"
	"github.com/elastic/go-elasticsearch/esapi"
	"github.com/google/uuid"
)

// NewElasticSearchIndexer creates a text indexer that uses an in-memory
// bleve instance for indexing documents.
func NewElasticSearchIndexer(esNodes []string, syncUpdates bool) (*ElasticSearchIndexer, error) {
	cfg := elasticsearch.Config{
		Addresses: esNodes,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	if err = ensureIndex(es); err != nil {
		return nil, err
	}

	refreshOpt := es.Update.WithRefresh("false")
	if syncUpdates {
		refreshOpt = es.Update.WithRefresh("true")
	}

	return &ElasticSearchIndexer{
		es:         es,
		refreshOpt: refreshOpt,
	}, nil
}

// Index inserts a new document to the index or updates the index entry
// for and existing document.
func (i *ElasticSearchIndexer) Index(doc *index.Document) error {
	if doc.LinkID == uuid.Nil {
		return fmt.Errorf("index: %w", index.ErrMissingLinkID)
	}

	var (
		buf   bytes.Buffer
		esDoc = makeEsDoc(doc)
	)
	update := map[string]interface{}{
		"doc":           esDoc,
		"doc_as_upsert": true,
	}
	if err := json.NewEncoder(&buf).Encode(update); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	res, err := i.es.Update(indexName, esDoc.LinkID, &buf, i.refreshOpt)
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}

	var updateRes esUpdateRes
	if err = unmarshalResponse(res, &updateRes); err != nil {
		return fmt.Errorf("index: %w", err)
	}

	return nil
}

// FindByID looks up a document by its link ID.
func (i *ElasticSearchIndexer) FindByID(linkID uuid.UUID) (*index.Document, error) {
	var buf bytes.Buffer
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"match": map[string]interface{}{
				"LinkID": linkID.String(),
			},
		},
		"from": 0,
		"size": 1,
	}
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	searchRes, err := runSearch(i.es, query)
	if err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	if len(searchRes.Hits.HitList) != 1 {
		return nil, fmt.Errorf("find by ID: %w", index.ErrNotFound)
	}

	return mapEsDoc(&searchRes.Hits.HitList[0].DocSource), nil
}

// Search the index for a particular query and return back a result
// iterator.
func (i *ElasticSearchIndexer) Search(q index.Query) (index.Iterator, error) {
	var qtype string
	switch q.Type {
	case index.QueryTypePhrase:
		qtype = "phrase"
	default:
		qtype = "best_fields"
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"function_score": map[string]interface{}{
				"query": map[string]interface{}{
					"multi_match": map[string]interface{}{
						"type":   qtype,
						"query":  q.Expression,
						"fields": []string{"Title", "Content"},
					},
				},
				"script_score": map[string]interface{}{
					"script": map[string]interface{}{
						"source": "_score + doc['PageRank'].value",
					},
				},
			},
		},
		"from": q.Offset,
		"size": batchSize,
	}

	searchRes, err := runSearch(i.es, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return &esIterator{es: i.es, searchReq: query, rs: searchRes, cumIdx: q.Offset}, nil
}

// UpdateScore updates the PageRank score for a document with the
// specified link ID. If no such document exists, a placeholder
// document with the provided score will be created.
func (i *ElasticSearchIndexer) UpdateScore(linkID uuid.UUID, score float64) error {
	var buf bytes.Buffer
	update := map[string]interface{}{
		"doc": map[string]interface{}{
			"LinkID":   linkID.String(),
			"PageRank": score,
		},
		"doc_as_upsert": true,
	}
	if err := json.NewEncoder(&buf).Encode(update); err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	res, err := i.es.Update(indexName, linkID.String(), &buf, i.refreshOpt)
	if err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	var updateRes esUpdateRes
	if err = unmarshalResponse(res, &updateRes); err != nil {
		return fmt.Errorf("update score: %w", err)
	}

	return nil
}

func ensureIndex(es *elasticsearch.Client) error {
	mappingsReader := strings.NewReader(esMappings)
	res, err := es.Indices.Create(indexName, es.Indices.Create.WithBody(mappingsReader))
	if err != nil {
		return fmt.Errorf("cannot create ES index: %w", err)
	} else if res.IsError() {
		err := unmarshalError(res)
		if esErr, valid := err.(esError); valid && esErr.Type == "resource_already_exists_exception" {
			return nil
		}
		return fmt.Errorf("cannot create ES index: %w", err)
	}

	return nil
}

func runSearch(es *elasticsearch.Client, searchQuery map[string]interface{}) (*esSearchRes, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(searchQuery); err != nil {
		return nil, fmt.Errorf("find by ID: %w", err)
	}

	// Perform the search request.
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(indexName),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}

	var esRes esSearchRes
	if err = unmarshalResponse(res, &esRes); err != nil {
		return nil, err
	}

	return &esRes, nil
}

func unmarshalError(res *esapi.Response) error {
	return unmarshalResponse(res, nil)
}

func unmarshalResponse(res *esapi.Response, to interface{}) error {
	defer func() { _ = res.Body.Close() }()

	if res.IsError() {
		var errRes esErrorRes
		if err := json.NewDecoder(res.Body).Decode(&errRes); err != nil {
			return err
		}

		return errRes.Error
	}

	return json.NewDecoder(res.Body).Decode(to)
}

func mapEsDoc(d *esDoc) *index.Document {
	return &index.Document{
		LinkID:    uuid.MustParse(d.LinkID),
		URL:       d.URL,
		Title:     d.Title,
		Content:   d.Content,
		IndexedAt: d.IndexedAt.UTC(),
		PageRank:  d.PageRank,
	}
}

func makeEsDoc(d *index.Document) esDoc {
	// Note: we intentionally skip PageRank as we don't want updates to
	// overwrite existing PageRank values.
	return esDoc{
		LinkID:    d.LinkID.String(),
		URL:       d.URL,
		Title:     d.Title,
		Content:   d.Content,
		IndexedAt: d.IndexedAt.UTC(),
	}
}
