package main

import "archive/zip"
import "regexp"

type zipFilesList []*zip.File

func (z zipFilesList) FindByRe(re *regexp.Regexp) (file *zip.File, ok bool) {
	for _, f := range z {
		if re.MatchString(f.Name) {
			file, ok = f, true
			return
		}
	}
	return
}
