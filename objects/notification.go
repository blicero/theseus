// /home/krylon/go/src/github.com/blicero/theseus/objects/notification.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-06-30 22:25:55 krylon>

// Package objects provides the data types used by the application.
package objects

import "time"

// Notification is the common interface for items the user should be
// notified about.
type Notification interface {
	Due() time.Time
	IsDue() bool
	Payload() (string, string)
}
