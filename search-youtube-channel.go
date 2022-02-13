package main

import (
	"flag"
	"fmt"
	"github.com/asticode/go-astisub"
	"github.com/blevesearch/bleve/v2"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"
)

func AddSubtitleItem(sb *strings.Builder, item *astisub.Item) {
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

type TranscriptionSegment struct {
	StartTime time.Duration
	EndTime   time.Duration
	EndPos    int
}

// bleve does not support time.Duration or int, only float so segment information must be stored as floats.
func (t TranscriptionSegment) ToFloats() (float64, float64, float64) {
	return t.StartTime.Seconds(), t.EndTime.Seconds(), float64(t.EndPos)
}

type VideoTranscription struct {
	Words    string
	Segments []float64
}

// Tells bleve what type of document a VideoTranscription is.
func (VideoTranscription) BleveType() string {
	return "VideoTranscription"
}

func ParseSubtitleFile(filename string) (*VideoTranscription, error) {
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
		AddSubtitleItem(&sb, item)
		f1, f2, f3 := TranscriptionSegment{item.StartAt, item.EndAt, sb.Len()}.ToFloats()
		segments = append(segments, f1, f2, f3)
	}
	return &VideoTranscription{sb.String(), segments}, nil
}

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
	mapping.AddDocumentMapping("VideoTranscription", vtmap) // This is where VideoTranscription.BleveType is pertinent.
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

func QueryIndex(query string, index bleve.Index) (*bleve.SearchResult, error) {
	request := bleve.NewSearchRequest(bleve.NewMatchQuery(query))
	request.Fields = []string{"Segments"}
	request.IncludeLocations = true
	return index.Search(request)
}

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

	res, err := QueryIndex(textQuery, index)
	perhapsExit(err, 4)

	for _, hit := range res.Hits {
		fmt.Printf("https://www.youtube.com/watch?v=%s (%.3f)\n", hit.ID, hit.Score)
	}
}
