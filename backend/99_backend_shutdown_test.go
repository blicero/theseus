// /home/krylon/go/src/github.com/blicero/theseus/backend/99_backend_shutdown_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 06. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-06 20:37:59 krylon>

package backend

import "testing"

func TestBanish(t *testing.T) {
	if back == nil {
		t.SkipNow()
	} else if !back.IsAlive() {
		t.SkipNow()
	}

	var err error

	if err = back.Banish(); err != nil {
		t.Errorf("Failed to banish Daemon: %s", err.Error())
	}
} // func TestBanish(t *testing.T)
