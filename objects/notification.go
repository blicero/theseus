// /home/krylon/go/src/github.com/blicero/theseus/objects/notification.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-17 18:27:37 krylon>

// Package objects provides the data types used by the application.
package objects

import "time"

// Notification represents one point in time when a Reminder is displayed.
// We use it to keep track of whether the Notification has been acknowledged
// yet, so we do not forget a Notification, but do not display it twice, either.
type Notification struct {
	ID           int64
	ReminderID   int64
	Timestamp    time.Time
	Displayed    time.Time
	Acknowledged time.Time
}
