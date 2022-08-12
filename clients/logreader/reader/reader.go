// /home/krylon/go/src/github.com/blicero/theseus/clients/logreader/reader/reader.go
// -*- mode: go; coding: utf-8; -*-
// Created on 25. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-27 03:35:14 krylon>

package reader

import (
	"log"
	"time"
)

type LogRecord struct {
	Timestamp time.Time
	Source    string
	Message   string
	Hash      string
}

type LogReader interface {
	GetLogger() *log.Logger
	GetRecords(start time.Time) ([]LogRecord, error)
}
