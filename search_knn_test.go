// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestKnnSearchSource(t *testing.T) {
	knn := NewKnnSearch("embedding", []float32{0.5, -1}, 10).
		NumCandidates(100).
		Filter(NewTermQuery("country", "DE")).
		Similarity(0.25).
		RescoreVectorOversample(2)
	src, err := NewSearchSource().Size(10).Knn(knn).Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"knn":{"field":"embedding","filter":{"term":{"country":"DE"}},"k":10,"num_candidates":100,"query_vector":[0.5,-1],"rescore_vector":{"oversample":2},"similarity":0.25},"size":10}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestKnnSearchSourceMinimal(t *testing.T) {
	src, err := NewSearchSource().Knn(NewKnnSearch("embedding", []float32{1}, 3)).Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"knn":{"field":"embedding","k":3,"query_vector":[1]}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
