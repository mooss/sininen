package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/asticode/go-astisub"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

////////////////////////////////////////////////
// Parsing and manipulation of transcriptions //
////////////////////////////////////////////////
// addSubtitleItem concatenates the content of a subtitle item into a string builder.
func addSubtitleItem(sb *strings.Builder, item *astisub.Item) {
	for i, line := range item.Lines {
		if i > 0 {
			sb.WriteRune(' ')
		}
		for j, litem := range line.Items {
			if j > 0 {
				sb.WriteRune(' ')
			}
			sb.WriteString(litem.Text)
		}
	}
}

// TranscriptionSegment records the temporal and textual position of a segment within an audio transcription.
type TranscriptionSegment struct {
	StartTime time.Duration
	EndTime   time.Duration
	EndPos    int
}

// bleve does not support time.Duration or int, only float so segment information must be stored as floats.
func (t TranscriptionSegment) ToFloats() (float64, float64, float64) {
	return t.StartTime.Seconds(), t.EndTime.Seconds(), float64(t.EndPos)
}

type Transcription struct {
	Words    string
	Segments []float64
}

// Tells bleve what type of document a Transcription is.
func (Transcription) BleveType() string {
	return "Transcription"
}

func ParseSubtitleFile(filename string) (*Transcription, error) {
	st, err := astisub.OpenFile(filename)
	if err != nil {
		return nil, err
	}

	segments := make([]float64, 0, 3*len(st.Items))
	var sb strings.Builder
	for i, item := range st.Items {
		if i > 0 {
			sb.WriteRune('\n')
		}
		addSubtitleItem(&sb, item)
		f1, f2, f3 := TranscriptionSegment{item.StartAt, item.EndAt, sb.Len()}.ToFloats()
		segments = append(segments, f1, f2, f3)
	}
	return &Transcription{sb.String(), segments}, nil
}

///////////////////
// bleve helpers //
///////////////////
func CreateSubtitleIndex(folder, lang string) (bleve.Index, error) {
	files, err := ioutil.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	// Define how to index and store data.
	segmentsMap := bleve.NewNumericFieldMapping()
	segmentsMap.Store = true
	segmentsMap.Index = false
	vtmap := bleve.NewDocumentMapping()
	vtmap.AddFieldMappingsAt("Segments", segmentsMap) // Default mapping is good enough for Words.
	mapping := bleve.NewIndexMapping()
	mapping.DefaultAnalyzer = lang
	mapping.AddDocumentMapping("Transcription", vtmap) // This is where Transcription.BleveType is pertinent.
	index, err := bleve.New(path.Join(folder, lang+".bleve"), mapping)
	if err != nil {
		return nil, err
	}

	// Index and store data.
	for _, file := range files {
		splitted := strings.Split(file.Name(), ".")
		if len(splitted) <= 2 || splitted[len(splitted)-2] != lang {
			continue
		}
		filepath := path.Join(folder, file.Name())
		document, err := ParseSubtitleFile(filepath)
		if err == nil {
			index.Index(splitted[0], document)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	return index, nil
}

func OpenSubtitleIndex(folder, lang string) (bleve.Index, error) {
	return bleve.Open(path.Join(folder, lang+".bleve"))
}

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

//////////////////////////
// Local utils and main //
//////////////////////////
func perhapsExit(err error, code int) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}

func main() {
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s channel-id search-query\n\nchannel-id must have been downloaded with the script download-channel-subtitles.sh.\n", os.Args[0])
	}

	channelName := flag.Arg(0)
	textQuery := flag.Arg(1)
	subtitlesFolder := path.Join("subtitles", channelName)
	info, err := os.Stat(subtitlesFolder)
	perhapsExit(err, 1)
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "%s is not a dir.\n", subtitlesFolder)
		os.Exit(2)
	}

	lang := "en"
	index, err := OpenSubtitleIndex(subtitlesFolder, lang)
	if err != nil {
		index, err = CreateSubtitleIndex(subtitlesFolder, lang)
	}
	perhapsExit(err, 3)

	raw, err := TextQuery(textQuery, index)
	perhapsExit(err, 4)

	videos, err := AssembleSearchResults(raw)
	perhapsExit(err, 5)

	for _, video := range videos {
		for _, hit := range video.Segments {
			fmt.Printf("https://www.youtube.com/watch?v=%s&t=%vs (%v terms, score=%.3f)\n",
				video.ID, int(hit.StartTime.Seconds()), hit.NTerms, video.Score)
		}
	}
}
