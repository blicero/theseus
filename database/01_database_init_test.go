// /home/krylon/go/src/github.com/blicero/theseus/database/01_database_init_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-01 19:28:05 krylon>

package database

import (
	"testing"

	"github.com/blicero/theseus/common"
)

var db *Database

func TestCreateDatabase(t *testing.T) {
	var err error

	if db, err = Open(common.DbPath); err != nil {
		db = nil
		t.Fatalf("Cannot open database at %s: %s",
			common.DbPath,
			err.Error())
	}
} // func TestCreateDatabase(t *testing.T)

// We prepare each query once to make sure there are no syntax errors in the SQL.
func TestPrepareQueries(t *testing.T) {
	if db == nil {
		t.SkipNow()
	}

	for id := range dbQueries {
		var err error
		if _, err = db.getQuery(id); err != nil {
			t.Errorf("Cannot prepare query %s: %s",
				id,
				err.Error())
		}
	}
} // func TestPrepareQueries(t *testing.T)
