package clio

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/davidmz/mustbe"
	"github.com/juju/errors"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var urlLikeRe = regexp.MustCompile(`^(https?|ftp)://`)

func deHTML(text string) (result string, links []string) {
	var (
		inAnchor bool
		aText    string
		aTitle   string
		aHref    string
	)
	z := html.NewTokenizer(strings.NewReader(text))
	for {
		tt := z.Next()
		switch tt {

		case html.ErrorToken:
			return

		case html.StartTagToken:
			t := z.Token()
			if t.DataAtom != atom.A {
				mustbe.OK(errors.Errorf("unexpected HTML tag %q", t.DataAtom))
			}
			inAnchor = true
			aText = ""
			aTitle = ""
			aHref = ""
			for _, a := range t.Attr {
				switch a.Key {
				case "title":
					aTitle = a.Val
				case "href":
					aHref = a.Val
				}
			}

		case html.TextToken:
			t := z.Token()
			if inAnchor {
				aText += t.Data
			} else {
				result += t.Data
			}

		case html.EndTagToken:
			inAnchor = false
			if !urlLikeRe.MatchString(aText) {
				result += aText
			} else if aTitle != "" {
				links = append(links, aTitle)
				result += aTitle
			} else {
				url := unshorten(aHref)
				links = append(links, url)
				result += url
			}
		}
	}
}

func unshorten(u string) string {
	isShort := strings.HasPrefix(u, "http://t.co/")
	if !isShort {
		return u
	}
	resp, err := http.Get(u) // not http.Head because of https://github.com/golang/go/issues/19895
	if err != nil {
		return u
	}
	resp.Body.Close()
	return resp.Request.URL.String()
}
