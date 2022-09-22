// /home/krylon/go/src/github.com/blicero/theseus/objects/01_recur_test.go
// -*- mode: go; coding: utf-8; -*-
// Created on 14. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-22 18:31:51 krylon>

package objects

import (
	"testing"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/objects/repeat"
)

func TestDue(t *testing.T) {
	type testCase struct {
		r         Reminder
		ref       time.Time
		expectDue time.Time
	}

	var (
		zero  = time.Unix(0, 0)
		now   = time.Now().Truncate(time.Minute)
		today = time.Now().Truncate(time.Second * 86400)
	)

	var cases = []testCase{
		testCase{
			r: Reminder{
				Title:     "Test01",
				Timestamp: zero.Add(time.Hour * 14),
				Recur: Recurrence{
					Repeat: repeat.Daily,
				},
			},
			ref:       today,
			expectDue: today.Add(time.Hour * 14),
		},
		testCase{
			r: Reminder{
				Title:     "Test02",
				Timestamp: zero.Add(time.Second * 27000), // 07:30
				Recur: Recurrence{
					Repeat: repeat.Custom,
					Days: Weekdays{
						false,
						true,
						false,
						true,
						false,
						true,
						false,
					},
				},
			},
			ref:       time.Date(2022, 9, 14, 0, 0, 0, 0, time.UTC),
			expectDue: time.Date(2022, 9, 15, 7, 30, 0, 0, time.UTC),
		},
		testCase{
			r: Reminder{
				Title:     "Test03",
				Timestamp: now.Add(time.Hour * 24),
				Recur: Recurrence{
					Repeat: repeat.Once,
				},
			},
			ref:       today,
			expectDue: now.Add(time.Hour * 24),
		},
		testCase{
			r: Reminder{
				Title:     "Test04",
				Timestamp: zero.Add(time.Hour * 8),
				Recur: Recurrence{
					Repeat: repeat.Custom,
					Days: Weekdays{
						true,
						true,
						true,
						true,
						true,
						false,
						false,
					},
				},
			},
			ref:       time.Date(2022, 9, 9, 15, 33, 0, 0, time.UTC),
			expectDue: time.Date(2022, 9, 12, 8, 0, 0, 0, time.UTC),
		},
	}

	for _, c := range cases {
		var due = c.r.DueNext(&c.ref)

		if delta := due.Truncate(time.Minute).Sub(c.expectDue.Truncate(time.Minute)); delta > time.Minute {
			t.Errorf(`Unexpected due time from Test case %s:
Expected:       %s
Got:            %s
Delta:          %s
`,
				c.r.Title,
				c.expectDue.Format(common.TimestampFormat),
				due.Format(common.TimestampFormat),
				delta)

		}
	}
} // func TestDue(t *testing.T)
