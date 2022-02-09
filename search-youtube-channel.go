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

func SubtitleFileContent(filename string) (string, error) {
	st, err := astisub.OpenFile(filename)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, item := range st.Items {
		if i > 0 {
			sb.WriteRune('\n')
		}
		AddSubtitleItem(&sb, item)
	}
	return sb.String(), nil
}

func CreateSubtitleIndex(folder, lang string) (bleve.Index, error) {
	files, err := ioutil.ReadDir(folder)
	if err != nil {
		return nil, err
	}

	mapping := bleve.NewIndexMapping()
	mapping.DefaultAnalyzer = lang
	index, err := bleve.New(path.Join(folder, lang+".bleve"), mapping)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		splitted := strings.Split(file.Name(), ".")
		if len(splitted) <= 2 || splitted[len(splitted)-2] != lang {
			continue
		}
		filepath := path.Join(folder, file.Name())
		content, err := SubtitleFileContent(filepath)

		if err == nil {
			index.Index(splitted[0], content)
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
	return index.Search(bleve.NewSearchRequest(bleve.NewMatchQuery(query)))
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

	index, err := OpenSubtitleIndex(subtitlesFolder, "en")
	if err != nil {
		index, err = CreateSubtitleIndex(subtitlesFolder, "en")
	}
	perhapsExit(err, 3)

	res, err := QueryIndex(textQuery, index)
	perhapsExit(err, 4)

	for _, hit := range res.Hits {
		fmt.Printf("https://www.youtube.com/watch?v=%s (%.3f)\n", hit.ID, hit.Score)
	}
}
