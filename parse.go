package sininen

import (
	"strings"
	"time"

	"github.com/asticode/go-astisub"
)

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

// transcriptionSegment records the temporal and textual position of a segment within an audio transcription.
type transcriptionSegment struct {
	StartTime time.Duration
	EndTime   time.Duration
	EndPos    int
}

// bleve does not support time.Duration or int, only float so segment information must be stored as floats.
func (t transcriptionSegment) toFloats() (float64, float64, float64) {
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
		f1, f2, f3 := transcriptionSegment{item.StartAt, item.EndAt, sb.Len()}.toFloats()
		segments = append(segments, f1, f2, f3)
	}
	return &Transcription{sb.String(), segments}, nil
}
