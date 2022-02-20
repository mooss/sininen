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

/////////////////////////////
// Search results assembly //
/////////////////////////////
// That is to say going from raw bleve results to results catered for audio transcriptions.

// SegmentHit represents a transcription segment that matched with a search query.
type SegmentHit struct {
	StartTime time.Duration
	EndTime   time.Duration
	Score     float64
	Terms     []string // Terms in the segment that matched with the search query.
}

// SearchResult represents a transcription file that matched with a search query.
type SearchResult struct {
	ID       string
	Score    float64
	Segments []SegmentHit // Segments that matched with the search query.
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
						cachedHit.Score += hit.Score
						cachedHit.Terms = append(cachedHit.Terms, term)
					} else {
						// segmentHit, err := newSegmentHit(segments, i, hit.Score, term)
						hitCache[i] = &SegmentHit{
							StartTime: start,
							EndTime:   end,
							Score:     hit.Score,
							Terms:     []string{term},
						}
					}
				}
			}
		}

		sortedSegments := make([]SegmentHit, 0, len(hitCache))
		for _, el := range hitCache {
			sort.Strings(el.Terms) // For consistency from one search to the next.
			sortedSegments = append(sortedSegments, *el)
		}
		sort.Slice(sortedSegments, func(i, j int) bool {
			si, sj := sortedSegments[i], sortedSegments[j]
			if len(si.Terms) != len(sj.Terms) { // To ensure stability of the sorting operation.
				return len(si.Terms) > len(sj.Terms)
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
