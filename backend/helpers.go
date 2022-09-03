// /home/krylon/go/src/github.com/blicero/theseus/backend/helpers.go
// -*- mode: go; coding: utf-8; -*-
// Created on 19. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-02 21:40:58 krylon>

package backend

import (
	"fmt"
	"time"

	"github.com/grandcat/zeroconf"
)

func durAbs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
} // func durAbs(d time.Duration) time.Duration

func rrStr(rr *zeroconf.ServiceEntry) string {
	// return fmt.Sprintf("%q @ %s.%s%s:%d",
	// 	rr.Instance,
	// 	rr.Service,
	// 	rr.HostName,
	// 	rr.Domain,
	// 	rr.Port)
	return fmt.Sprintf("%s:%d",
		rr.HostName,
		rr.Port)
} // func rrStr(rr *zeroconf.ServiceEntry) string
