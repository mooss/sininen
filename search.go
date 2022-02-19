package sininen

import (
	"errors"
	"fmt"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"sort"
	"time"
)

///////////////////
// bleve helpers //
///////////////////
func TextQuery(query string, index bleve.Index) (*bleve.SearchResult, error) {
	request := bleve.NewSearchRequest(bleve.NewMatchQuery(query))
	request.Fields = []string{"Segments"}
	request.IncludeLocations = true
	return index.Search(request)
}

/////////////////////////////
// Search results assembly //
/////////////////////////////
// That is to say going from raw bleve results to results catered for audio transcriptions.
type SegmentHit struct {
	StartTime time.Duration
	EndTime   time.Duration
	NTerms    int
}

type SearchResult struct {
	ID       string
	Score    float64
	Segments []SegmentHit
}

// locateSegment returns the index of the segment containing the given location.
func locateSegment(segments []interface{}, location *search.Location) int {
	searchFailed := false
	position := sort.Search(len(segments)/3, func(i int) bool {
		endPos, isFloat := segments[i*3+2].(float64)
		if isFloat && endPos > 0 {
			return uint64(endPos) > location.Start
		}
		searchFailed = true
		return false
	})
	if searchFailed {
		return -1
	}
	return position
}

// newSegmentHit builds a SegmentInt by extracting information from a serialized segment array.
func newSegmentHit(segments []interface{}, segmentPos int) (*SegmentHit, error) {
	extract := func(i int) (float64, error) {
		value, valid := segments[i].(float64)
		if !valid {
			return -1, fmt.Errorf("Expected segments[%v] to be of type float64, got %T.", i, segments[i])
		}
		return value, nil
	}

	floatStart, err := extract(segmentPos * 3)
	if err != nil {
		return nil, err
	}
	floatEnd, err := extract(segmentPos*3 + 1)
	if err != nil {
		return nil, err
	}
	return &SegmentHit{
		StartTime: time.Duration(int(floatStart) * int(time.Second)),
		EndTime:   time.Duration(int(floatEnd) * int(time.Second)),
		NTerms:    1,
	}, nil
}

// AssembleSearchResults builds transcription search results with timestamp information using raw bleve search results.
func AssembleSearchResults(bleveResults *bleve.SearchResult) ([]SearchResult, error) {
	result := []SearchResult{}
	for _, hit := range bleveResults.Hits {
		raw, exists := hit.Fields["Segments"]
		if !exists {
			return nil, errors.New("Segments are missing from bleve search results.")
		}
		segments, valid := raw.([]interface{})
		if !valid {
			return nil, fmt.Errorf("Segments should be an array, got %T", raw)
		}
		if len(segments)%3 != 0 {
			return nil, fmt.Errorf("Serialized segments should be a multiple of 3, got %v segments.", len(segments))
		}

		// Segment hits are cached because search hits for different terms can orrur in the same segment.
		hitCache := map[int]*SegmentHit{}
		for _, locationMap := range hit.Locations {
			for _, locations := range locationMap {
				for _, location := range locations {
					i := locateSegment(segments, location)
					if i < 0 {
						return nil, errors.New("Failed to locate segment.")
					}
					segmentHit, err := newSegmentHit(segments, i)
					if err != nil {
						return nil, err
					}
					cachedHit, isCached := hitCache[i]
					if isCached {
						cachedHit.NTerms++
					} else {
						hitCache[i] = segmentHit
					}
				}
			}
		}

		sortedSegments := make([]SegmentHit, 0, len(hitCache))
		for _, el := range hitCache {
			sortedSegments = append(sortedSegments, *el)
		}
		sort.Slice(sortedSegments, func(i, j int) bool {
			si, sj := sortedSegments[i], sortedSegments[j]
			if si.NTerms != sj.NTerms { // To ensure stability of the sorting operation.
				return si.NTerms > sj.NTerms
			}
			return si.StartTime < sj.StartTime

		})

		result = append(result, SearchResult{
			ID:       hit.ID,
			Score:    hit.Score,
			Segments: sortedSegments,
		})
	}
	return result, nil
}
