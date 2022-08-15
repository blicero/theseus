// /home/krylon/go/src/github.com/blicero/theseus/clients/logreader/reader/reader/reader_linux.go
// -*- mode: go; coding: utf-8; -*-
// Created on 27. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-13 20:41:26 krylon>

// Package reader provides Reader struct which knows how to read the log
// files on various Unix systems.
package reader

import (
	"log"
	"regexp"
)

// Reader bla
type Reader struct {
	log *log.Logger
	pat []*regexp.Regexp
	srv string
}
