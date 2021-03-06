// /home/krylon/go/src/github.com/blicero/theseus/database/dbqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-06 18:51:25 krylon>

package database

import "github.com/blicero/theseus/database/query"

var dbQueries = map[query.ID]string{
	query.ReminderAdd: `
INSERT INTO reminder (title, description, due, uuid)
VALUES               (    ?,           ?,   ?,    ?)
`,
	query.ReminderDelete: "DELETE FROM reminder WHERE id = ?",
	query.ReminderGetPending: `
SELECT
    id,
    title,
    description,
    due,
    uuid
FROM reminder
WHERE NOT finished
`,
	query.ReminderGetFinished: `
SELECT
    id,
    title,
    description,
    due,
    uuid
FROM reminder
WHERE finished
`,
	query.ReminderGetAll: `
SELECT
    id,
    title,
    description,
    due,
    finished,
    uuid
FROM reminder
`,
	query.ReminderGetByID: `
SELECT
    title,
    description,
    due,
    finished,
    uuid
FROM reminder
WHERE id = ?
`,
	query.ReminderSetTitle:       "UPDATE reminder SET title = ? WHERE id = ?",
	query.ReminderSetDescription: "UPDATE reminder SET description = ? WHERE id = ?",
	query.ReminderSetTimestamp:   "UPDATE reminder SET due = ? WHERE id = ?",
	query.ReminderSetFinished:    "UPDATE reminder SET finished = ? WHERE id = ?",
}
