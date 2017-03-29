package main

import (
	"archive/zip"
	"encoding/json"
	"io/ioutil"

	"github.com/juju/errors"
)

func readZipObject(file *zip.File, v interface{}) error {
	r, err := file.Open()
	if err != nil {
		return errors.Annotate(err, "cannot open archived file")
	}
	defer r.Close()

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Annotate(err, "cannot read archived file")
	}

	err = json.Unmarshal(data, v)
	if err != nil {
		return errors.Annotate(err, "cannot parse JSON")
	}

	return nil
}
