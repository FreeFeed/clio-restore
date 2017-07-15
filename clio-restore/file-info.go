package main

import (
	"archive/zip"
	"regexp"
)

// general (not image) file info

type fileInfo struct {
	zipFile     *zip.File
	ContentType string
	Name        string
}

var supportedExtensions = regexp.MustCompile(`(?i)\.(jpe?g|png|gif|mp3|m4a|ogg|wav|txt|pdf|docx?|pptx?|xlsx?)$`)

var audioTypes = map[string]string{
	"audio/mpeg":  "mp3",
	"audio/x-m4a": "m4a",
	"audio/mp4":   "m4a",
	"audio/ogg":   "ogg",
	"audio/x-wav": "wav",
}

func (fi *fileInfo) attachType() string {
	if _, ok := audioTypes[fi.ContentType]; ok {
		return "audio"
	}
	return "general"
}

func (fi *fileInfo) ext() string {
	if ext, ok := audioTypes[fi.ContentType]; ok {
		return ext
	}
	m := supportedExtensions.FindStringSubmatch(fi.Name)
	if m != nil {
		return m[1]
	}
	return ""
}

func (fi *fileInfo) dotExt() string {
	if ext := fi.ext(); ext != "" {
		return "." + ext
	}
	return ""
}

func (fi *fileInfo) isMP3() bool { return fi.ContentType == "audio/mpeg" }

func (fi *fileInfo) size() int { return int(fi.zipFile.FileHeader.UncompressedSize64) }
