package config

import (
	"flag"
	"path/filepath"

	"gopkg.in/gcfg.v1"
)
import "os"

// Config holds program configuration taken from ini file
type Config struct {
	DbStr        string
	GM           string
	GifSicle     string
	SRGB         string
	AttDir       string
	S3Bucket     string
	MP3Zip       string
	AttURL       string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPBcc      string
}

var fileName string

func init() {
	flag.StringVar(&fileName, "conf", "", "path to ini file (default is PROGRAM_DIR/clio.ini)")
}

// Load loads config file from -conf flag or (if flag is not set)
// from the default location PROGRAM_DIR/clio.ini
func Load() (*Config, error) {
	if fileName == "" {
		fileName = filepath.Join(filepath.Dir(os.Args[0]), "clio.ini")
	}
	conf := &struct{ Clio Config }{}
	if err := gcfg.ReadFileInto(conf, fileName); err != nil {
		return nil, err
	}
	return &conf.Clio, nil
}
