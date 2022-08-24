// /home/krylon/go/src/github.com/blicero/theseus/backend/helpers.go
// -*- mode: go; coding: utf-8; -*-
// Created on 19. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-24 21:10:32 krylon>

package backend

import "time"

func durAbs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
} // func durAbs(d time.Duration) time.Duration
