package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/mooss/sininen"
)

func perhapsExit(err error, code int) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}

func main() {
	jsonFlag := flag.Bool("json", false, "Output search results as JSON.")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s channel-id search-query [-json]\n\nchannel-id must have been downloaded with the script download-channel-subtitles.sh.\n", os.Args[0])
		os.Exit(6)
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
	index, err := sininen.OpenTranscriptionIndex(subtitlesFolder, lang)
	if err != nil {
		index, err = sininen.CreateSubtitleIndex(subtitlesFolder, lang)
	}
	perhapsExit(err, 3)

	raw, err := sininen.TextQuery(textQuery, index)
	perhapsExit(err, 4)

	videos, err := sininen.AssembleSearchResults(raw)
	perhapsExit(err, 5)

	scoredSegments := videos.ScoredSegments()
	if *jsonFlag {
		marshalledBytes, err := json.Marshal(scoredSegments)
		perhapsExit(err, 6)
		fmt.Println(string(marshalledBytes))
	} else {
		for _, segment := range scoredSegments {
			fmt.Printf("https://www.youtube.com/watch?v=%s&t=%vs (%v, score=%.3f)\n",
				segment.ID, int(segment.StartTime.Seconds()), segment.SortedTerms, segment.Score)
		}
	}
}
