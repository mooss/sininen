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
// TextQuery makes a plain text search against an transcription index.
func TextQuery(query string, index bleve.Index) (*bleve.SearchResult, error) {
	request := bleve.NewSearchRequest(bleve.NewMatchQuery(query))
	request.Fields = []string{"Segments"} // Include the Segments field without which the timestamps cannot be deduced.
	request.IncludeLocations = true
	return index.Search(request)
}

////////////////////////////////////
// Search results data structures //
////////////////////////////////////
// SegmentHit represents a transcription segment that matched with a search query.
type SegmentHit struct {
	StartTime   time.Duration `json:"start_time"`
	EndTime     time.Duration `json:"end_time"`
	SortedTerms []string      `json:"sorted_terms"` // Terms in the segment that matched with the search query, sorted in increasing order.
}

// NDistinctTerms returns the number of distinct terms in the segment that matched with the search query.
func (sh SegmentHit) NDistinctTerms() int {
	result := 0
	var last string
	for _, term := range sh.SortedTerms {
		if term != last {
			last = term
			result++
		}
	}
	return result
}

// SearchResult represents a transcription file that matched with a search query.
type SearchResult struct {
	ID       string
	Score    float64
	Segments []SegmentHit // Segments that matched with the search query.
}

// SearchResultSequence represents a sequence of transcription files that matched with a search query.
type SearchResultSequence []SearchResult

func (srs SearchResultSequence) lenSegments() int {
	result := 0
	for _, sr := range srs {
		result += len(sr.Segments)
	}
	return result
}

// ScoredSegment is a SegmentHit with its score and its transcription ID.
type ScoredSegment struct {
	SegmentHit
	Score float64 `json:"score"`
	ID    string  `json:"id"`
}

// ScoredSegments flattens a search results hierarchy by returning the scored segments, sorted by score.
func (srs SearchResultSequence) ScoredSegments() []ScoredSegment {
	result := make([]ScoredSegment, 0, srs.lenSegments())
	for _, sr := range srs {
		for _, segment := range sr.Segments {
			result = append(result, ScoredSegment{
				SegmentHit: segment,
				Score:      sr.Score * float64(segment.NDistinctTerms()),
				ID:         sr.ID,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		ri, rj := result[i], result[j]
		if ri.Score != rj.Score {
			return ri.Score > rj.Score
		}
		if ri.ID != rj.ID {
			return ri.ID < rj.ID
		}
		return ri.StartTime < rj.StartTime
	})
	return result
}

/////////////////////////////
// Search results assembly //
/////////////////////////////
// That is to say going from raw bleve results to results catered for audio transcriptions.

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

// extractDurations extracts duration information from a serialized segment array.
func extractDurations(segments []interface{}, segmentPos int) (startTime, endTime time.Duration, err error) {
	extract := func(i int) (float64, error) {
		value, valid := segments[i].(float64)
		if !valid {
			return -1, fmt.Errorf("Expected segments[%v] to be of type float64, got %T.", i, segments[i])
		}
		return value, nil
	}

	floatStart, err := extract(segmentPos * 3)
	if err != nil {
		return
	}
	floatEnd, err := extract(segmentPos*3 + 1)
	if err != nil {
		return
	}
	startTime = time.Duration(int(floatStart) * int(time.Second))
	endTime = time.Duration(int(floatEnd) * int(time.Second))
	return
}

// AssembleSearchResults builds transcription search results with timestamp information using raw bleve search results.
func AssembleSearchResults(bleveResults *bleve.SearchResult) (SearchResultSequence, error) {
	result := SearchResultSequence{}
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
			for term, locations := range locationMap {
				for _, location := range locations {
					i := locateSegment(segments, location)
					if i < 0 {
						return nil, errors.New("Failed to locate segment.")
					}
					start, end, err := extractDurations(segments, i)
					if err != nil {
						return nil, err
					}
					cachedHit, isCached := hitCache[i]
					if isCached {
						cachedHit.SortedTerms = append(cachedHit.SortedTerms, term) // Will sort later.
					} else {
						// segmentHit, err := newSegmentHit(segments, i, hit.Score, term)
						hitCache[i] = &SegmentHit{
							StartTime:   start,
							EndTime:     end,
							SortedTerms: []string{term},
						}
					}
				}
			}
		}

		sortedSegments := make([]SegmentHit, 0, len(hitCache))
		for _, el := range hitCache {
			sort.Strings(el.SortedTerms)
			sortedSegments = append(sortedSegments, *el)
		}
		sort.Slice(sortedSegments, func(i, j int) bool {
			si, sj := sortedSegments[i], sortedSegments[j]
			if len(si.SortedTerms) != len(sj.SortedTerms) { // To ensure stability of the sorting operation.
				return len(si.SortedTerms) > len(sj.SortedTerms)
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
