package hashtags

import (
	"regexp"
	"strings"
)

var (
	hashtagWord = `[^\x{0000}-\x{0020}\x{007F}\x{0080}-\x{00A0}\x{0021}-\x{002F}\x{003A}-\x{0040}` +
		`\x{005B}-\x{0060}\x{007B}-\x{007E}\x{00A1}-\x{00BF}\x{00D7}\x{00F7}\x{2000}\x{206F}]+`
	htRe = regexp.MustCompile(`(?:^|[^/?\w])#(` + hashtagWord + `(?:[_-]` + hashtagWord + `)*)`)
)

// Extract returns all unique low-cased hashtags found in text
func Extract(text string) (found []string) {
	s := htRe.FindAllStringSubmatch(text, -1)
	if len(s) == 0 {
		return
	}
	m := make(map[string]struct{})
	for _, h := range s {
		m[strings.ToLower(h[1])] = struct{}{}
	}
	for h := range m {
		found = append(found, h)
	}
	return
}
