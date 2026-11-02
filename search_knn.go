// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

type KnnSearch struct {
	field         string
	queryVector   []float32
	k             int
	numCandidates int
	filter        Query
	similarity    *float32
	oversample    float32
}

func NewKnnSearch(field string, queryVector []float32, k int) *KnnSearch {
	return &KnnSearch{field: field, queryVector: queryVector, k: k}
}

func (s *KnnSearch) NumCandidates(numCandidates int) *KnnSearch {
	s.numCandidates = numCandidates
	return s
}

func (s *KnnSearch) Filter(filter Query) *KnnSearch {
	s.filter = filter
	return s
}

func (s *KnnSearch) Similarity(similarity float32) *KnnSearch {
	s.similarity = &similarity
	return s
}

func (s *KnnSearch) RescoreVectorOversample(oversample float32) *KnnSearch {
	s.oversample = oversample
	return s
}

func (s *KnnSearch) Source() (interface{}, error) {
	src := map[string]interface{}{
		"field":        s.field,
		"query_vector": s.queryVector,
		"k":            s.k,
	}
	if s.numCandidates > 0 {
		src["num_candidates"] = s.numCandidates
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
	if s.oversample > 0 {
		src["rescore_vector"] = map[string]interface{}{"oversample": s.oversample}
	}
	return src, nil
}
