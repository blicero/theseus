// /home/krylon/go/src/github.com/blicero/theseus/database/initqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 30. 06. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-17 17:07:33 krylon>

package database

var initQueries = []string{
	`
CREATE TABLE reminder (
    id          INTEGER PRIMARY KEY,
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    due         INTEGER NOT NULL,
    finished    INTEGER NOT NULL DEFAULT 0,
    repeat      INTEGER NOT NULL DEFAULT 0,
    weekdays    INTEGER NOT NULL DEFAULT 0,
    counter     INTEGER NOT NULL DEFAULT 0,
    counter_max INTEGER NOT NULL DEFAULT 0,
    uuid        TEXT UNIQUE NOT NULL,
    changed     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (title, due),
    -- CHECK (due > 1656624376), -- 2022-06-30, ~23:26
    CHECK ((repeat = 0 AND due > 1656624376)
           OR ((repeat = 1 OR repeat = 2) AND
               (due BETWEEN 0 AND 86400))),
    CHECK (counter >= 0 AND counter_max >= 0 AND counter <= counter_max)

) STRICT
`,
	"CREATE INDEX reminder_due_idx ON reminder (due)",
	"CREATE INDEX reminder_finished_idx ON reminder (finished)",
	"CREATE INDEX reminder_uuid_idx ON reminder (uuid)",
	"CREATE INDEX reminder_changed_idx ON reminder (changed)",

	`
CREATE TABLE notification (
    id			INTEGER PRIMARY KEY,
    reminder_id		INTEGER NOT NULL,
    timestamp		INTEGER NOT NULL,
    displayed		INTEGER,
    acknowledged	INTEGER,
    UNIQUE (reminder_id, timestamp),
    CHECK (NOT (displayed IS NULL AND acknowledged IS NOT NULL)),
    FOREIGN KEY (reminder_id) REFERENCES reminder (id)
        ON UPDATE RESTRICT
        ON DELETE CASCADE
) STRICT
`,
	"CREATE INDEX rec_rem_idx ON notification (reminder_id)",
	"CREATE INDEX rec_time_idx ON notification (timestamp)",
	"CREATE INDEX rec_ack_idx ON notification (acknowledged)",
}
