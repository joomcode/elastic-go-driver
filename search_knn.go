// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

// KnnSearch is a top-level approximate kNN search on a dense_vector field.
// Unlike the knn query, it returns the global top k across shards.
// See https://www.elastic.co/docs/reference/elasticsearch/rest-apis/search-apis/knn-search-api
// for details.
type KnnSearch struct {
	field         string
	queryVector   []float32
	k             int
	numCandidates *int
	filter        Query
	similarity    *float32
	oversample    *float32
}

// NewKnnSearch creates a new KnnSearch.
func NewKnnSearch(field string, queryVector []float32, k int) *KnnSearch {
	return &KnnSearch{
		field:       field,
		queryVector: queryVector,
		k:           k,
	}
}

// NumCandidates is the number of nearest neighbor candidates to consider per shard.
func (s *KnnSearch) NumCandidates(numCandidates int) *KnnSearch {
	s.numCandidates = &numCandidates
	return s
}

// Filter restricts the documents that can match before the kNN search.
func (s *KnnSearch) Filter(filter Query) *KnnSearch {
	s.filter = filter
	return s
}

// Similarity is the minimum raw vector similarity for a document to match.
func (s *KnnSearch) Similarity(similarity float32) *KnnSearch {
	s.similarity = &similarity
	return s
}

// RescoreVectorOversample sets rescore_vector.oversample for quantized vectors.
func (s *KnnSearch) RescoreVectorOversample(oversample float32) *KnnSearch {
	s.oversample = &oversample
	return s
}

// Source generates the JSON serializable fragment for the KnnSearch.
func (s *KnnSearch) Source() (interface{}, error) {
	src := map[string]interface{}{
		"field":        s.field,
		"query_vector": s.queryVector,
		"k":            s.k,
	}
	if s.numCandidates != nil {
		src["num_candidates"] = *s.numCandidates
	}
	if s.filter != nil {
		filter, err := s.filter.Source()
		if err != nil {
			return nil, err
		}
		src["filter"] = filter
	}
	if s.similarity != nil {
		src["similarity"] = *s.similarity
	}
	if s.oversample != nil {
		src["rescore_vector"] = map[string]interface{}{"oversample": *s.oversample}
	}
	return src, nil
}
