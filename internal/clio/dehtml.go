package clio

import (
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

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

var shortDomains = []string{
	"t.co",
	"bit.ly",
	"bitly.com",
	"bitly.is",
	"j.mp",
	"goo.gl",
	"tinyurl.com",
	"ow.ly",
	"b23.ru",
}

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func unshorten(u string) string {
	pURL, err := url.Parse(u)
	if err != nil {
		return u
	}
	isShort := false
	for _, domain := range shortDomains {
		if pURL.Hostname() == domain {
			isShort = true
			break
		}
	}
	if !isShort {
		return u
	}

	u1 := strings.Replace(u, "://b23.ru/", "://z23.ru/", 1)

	// turn off log because of https://github.com/golang/go/issues/19895
	log.SetOutput(ioutil.Discard)
	resp, err := httpClient.Head(u1)
	log.SetOutput(os.Stderr)
	if err != nil {
		return u
	}
	resp.Body.Close()
	return resp.Request.URL.String()
}
