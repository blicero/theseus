// /home/krylon/go/src/github.com/blicero/theseus/database/dbqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-09 20:14:37 krylon>

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
	query.RecurrenceAdd: `
INSERT INTO recurrence (
reminder_id,
offset,
recur_type,
max_count,
counter,
weekdays,
uuid
)
VALUES (
 ?, ?, ?, ?, 0, ?, ?
)
`,
	query.RecurrenceDelete:    "DELETE FROM recurrence WHERE id = ?",
	query.RecurrenceSetOffset: "UPDATE recurrence SET offset = ? WHERE id = ?",
	query.RecurrenceSetMax:    "UPDATE recurrence SET max_count = ? WHERE id = ?",
	query.RecurrenceIsMax:     "SELECT (counter = max_count) FROM recurrence WHERE id = ?",
	query.RecurrenceIncCount:  "UPDATE recurrence SET counter = counter + 1 WHERE id = ? RETURNING counter",
	query.RecurrenceHasMax:    "SELECT (max_count > 0) FROM recurrence WHERE id = ?",
	query.RecurrenceGetForReminder: `
SELECT
    id,
    offset,
    recur_type,
    max_count,
    counter,
    weekdays,
    changed,
    uuid
FROM recurrence
WHERE reminder_id = ?
`,
	query.RecurrenceGetByWeekday: `
SELECT
    id,
    reminder_id,
    offset,
    recur_type,
    max_count,
    counter,
    weekdays,
    changed,
    uuid
FROM recurrence
WHERE (weekdays & (1 << ?)) <> 0
`,
}
