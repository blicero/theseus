// /home/krylon/go/src/github.com/blicero/theseus/database/initqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-06 21:52:44 krylon>

package database

var initQueries = []string{
	`
CREATE TABLE reminder (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    due         INTEGER NOT NULL,
    finished    INTEGER NOT NULL DEFAULT 0,
    uuid        TEXT UNIQUE NOT NULL,
    changed     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (title, due),
    CHECK (due > 1656624376) -- 2022-06-30, ~23:26
)
`,
	"CREATE INDEX reminder_due_idx ON reminder (due)",
	"CREATE INDEX reminder_finished_idx ON reminder (finished)",
	"CREATE INDEX reminder_uuid_idx ON reminder (uuid)",
	"CREATE INDEX reminder_changed_idx ON reminder (changed)",

	`
CREATE TABLE recurrence (
    id		INTEGER PRIMARY KEY,
    reminder_id INTEGER NOT NULL,
    offset      INTEGER NOT NULL,
    recur_type  INTEGER NOT NULL,
    max_count   INTEGER NOT NULL DEFAULT 0,
    counter     INTEGER NOT NULL DEFAULT 0,
    weekdays    INTEGER NOT NULL DEFAULT 0,
    changed     INTEGER NOT NULL,
    FOREIGN KEY (reminder_id) REFERENCES reminder (id),
    CHECK (offset BETWEEN 0 AND 86400),
    CHECK (counter <= max_count)
)
`,

	// 	`
	// CREATE TABLE recurrence (
	//     id          INTEGER PRIMARY KEY,
	//     reminder_id INTEGER NOT NULL,

	//     FOREIGN KEY (reminder_id) REFERENCES reminder (id)
	//         ON UPDATE RESTRICT
	//         ON DELETE CASCADE
	// )
	// `,
}
