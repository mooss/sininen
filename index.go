package sininen

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/blevesearch/bleve/v2"
)

// CreateSubtitleIndex opens, parses and indexes the subtitles file in the given folder and the given language.
// The created index is saved inside the folder.
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

// OpenTranscriptionIndex opens a stored subtitle index, such as the one created by CreateSubtitleIndex.
func OpenTranscriptionIndex(folder, lang string) (bleve.Index, error) {
	return bleve.Open(path.Join(folder, lang+".bleve"))
}
