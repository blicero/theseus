// /home/krylon/go/src/github.com/blicero/theseus/clients/logreader/main.go
// -*- mode: go; coding: utf-8; -*-
// Created on 25. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-16 23:11:28 krylon>

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/blicero/theseus/clients/logreader/reader"
	"github.com/blicero/theseus/common"
)

func main() {
	var (
		err error
		rdr reader.LogReader
	)

	if rdr, err = reader.NewReader("http://localhost:7202", "/var/log", "messages", "daemon"); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Cannot create LogReader: %s\n",
			err.Error())
		os.Exit(1)
	}

	var records []reader.LogRecord

	if records, err = rdr.GetRecords(time.Now().Add(time.Hour * -96)); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Failed to get Records: %s\n",
			err.Error(),
		)
		os.Exit(1)
	}

	for _, r := range records {
		fmt.Printf("%s - %s\n",
			r.Timestamp.Format(common.TimestampFormat),
			r.Message)
	}
}
