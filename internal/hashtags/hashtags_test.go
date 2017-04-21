package hashtags

import (
	"reflect"
	"sort"
	"testing"
)

var hashtagTestData = []struct {
	Text  string
	Found []string
}{
	{"#abc", []string{"abc"}},
	{"#abc #abc", []string{"abc"}},
	{"#abc#abc", []string{"abc"}},
	{"#abc #bca", []string{"abc", "bca"}},
	{"#abc,#bca", []string{"abc", "bca"}},
	{"#abc-bca #кошки_мышки #2", []string{"2", "abc-bca", "кошки_мышки"}},
	{"#abC #aBC", []string{"abc"}},
	{"12#abC", []string(nil)},
	{"http://example.com/a#abC", []string(nil)},
	{"http://example.com/#abC", []string(nil)},
	{"http://example.com/?#abC", []string(nil)},
}

func TestExtract(t *testing.T) {
	for _, d := range hashtagTestData {
		found := Extract(d.Text)
		sort.Strings(found)
		sort.Strings(d.Found)
		if !reflect.DeepEqual(found, d.Found) {
			t.Errorf("Extract from %q: got %v, expects %v", d.Text, found, d.Found)
		}
	}
}
