// /home/krylon/go/src/github.com/blicero/theseus/database/dbqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-31 21:19:26 krylon>

package database

import "github.com/blicero/theseus/database/query"

var dbQueries = map[query.ID]string{
	query.ReminderAdd: `
INSERT INTO reminder (title, description, due, uuid, changed)
VALUES               (    ?,           ?,   ?,    ?,       ?)
`,
	query.ReminderDelete: "DELETE FROM reminder WHERE id = ?",
	query.ReminderGetPending: `
SELECT
    id,
    title,
    description,
    due,
    uuid,
    changed
FROM reminder
WHERE finished = 0
  AND due < ?
ORDER BY finished, due, title
`,
	query.ReminderGetFinished: `
SELECT
    id,
    title,
    description,
    due,
    uuid,
    changed
FROM reminder
WHERE finished
ORDER BY finished, due, title
`,
	query.ReminderGetAll: `
SELECT
    id,
    title,
    description,
    due,
    finished,
    uuid,
    changed
FROM reminder
ORDER BY finished, due, title
`,
	query.ReminderGetByID: `
SELECT
    title,
    description,
    due,
    finished,
    uuid,
    changed
FROM reminder
WHERE id = ?
`,
	query.ReminderSetTitle: `
UPDATE reminder
SET title = ?, changed = ?
WHERE id = ?`,
	query.ReminderSetDescription: `
UPDATE reminder
SET description = ?, changed = ?
WHERE id = ?`,
	query.ReminderSetTimestamp: `
UPDATE reminder
SET due = ?, changed = ?
WHERE id = ?`,
	query.ReminderSetFinished: `
UPDATE reminder
SET finished = ?, changed = ?
WHERE id = ?`,
	query.ReminderReactivate: `
UPDATE reminder
SET finished = 0, due = ?, changed = ?
WHERE id = ?`,
	query.ReminderSetChanged: `
UPDATE reminder
SET changed = ?
WHERE id = ?
`,
}
