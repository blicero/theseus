// /home/krylon/go/src/github.com/blicero/theseus/database/dbqueries.go
// -*- mode: go; coding: utf-8; -*-
// Created on 01. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-13 17:54:57 krylon>

package database

import "github.com/blicero/theseus/database/query"

var dbQueries = map[query.ID]string{
	query.ReminderAdd: `
INSERT INTO reminder (title, description, due, repeat, weekdays, counter, counter_max, uuid, changed)
VALUES               (    ?,           ?,   ?,      ?,        ?,       ?,           ?,    ?,       ?)
`,
	query.ReminderDelete: "DELETE FROM reminder WHERE id = ?",
	query.ReminderGetPending: `
SELECT
    id,
    title,
    description,
    due,
    repeat,
    weekdays,
    counter,
    counter_max,
    uuid,
    changed
FROM reminder
WHERE finished = 0
ORDER BY due, title
`,
	query.ReminderGetFinished: `
SELECT
    id,
    title,
    description,
    due,
    repeat,
    weekdays,
    counter,
    counter_max,
    uuid,
    changed
FROM reminder
WHERE finished
ORDER BY due, title
`,
	query.ReminderGetAll: `
SELECT
    id,
    title,
    description,
    due,
    repeat,
    weekdays,
    counter,
    counter_max,
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
    repeat,
    weekdays,
    counter,
    counter_max,
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
	query.ReminderSetRepeat: `
UPDATE reminder
SET repeat = ?, changed = ?
WHERE id = ?
`,
	query.ReminderSetWeekdays: `
UPDATE reminder
SET weekdays = ?, changed = ?
WHERE id = ?
`,
	query.ReminderSetLimit: `
UPDATE reminder
SET
    counter_max = ?,
    counter = MIN(counter, counter_max),
    changed = ?
WHERE id = ?
`,
	query.ReminderResetCounter: `
UPDATE reminder
SET counter = 0, changed = ?
WHERE id = ?
`,
	query.ReminderIncCounter: `
UPDATE reminder
SET counter = counter + 1, changed = ?
WHERE id = ?
RETURNING counter
`,
	query.ReminderSetChanged: `
UPDATE reminder
SET changed = ?
WHERE id = ?
`,
	// 	query.RecurrenceAdd: `
	// INSERT INTO recurrence (
	// reminder_id,
	// offset,
	// recur_type,
	// max_count,
	// counter,
	// weekdays,
	// uuid
	// )
	// VALUES (
	//  ?, ?, ?, ?, 0, ?, ?
	// )
	// `,
	// 	query.RecurrenceDelete:    "DELETE FROM recurrence WHERE id = ?",
	// 	query.RecurrenceSetOffset: "UPDATE recurrence SET offset = ? WHERE id = ?",
	// 	query.RecurrenceSetMax:    "UPDATE recurrence SET max_count = ? WHERE id = ?",
	// 	query.RecurrenceIsMax:     "SELECT (counter = max_count) FROM recurrence WHERE id = ?",
	// 	query.RecurrenceIncCount:  "UPDATE recurrence SET counter = counter + 1 WHERE id = ? RETURNING counter",
	// 	query.RecurrenceHasMax:    "SELECT (max_count > 0) FROM recurrence WHERE id = ?",
	// 	query.RecurrenceGetForReminder: `
	// SELECT
	//     id,
	//     offset,
	//     recur_type,
	//     max_count,
	//     counter,
	//     weekdays,
	//     changed,
	//     uuid
	// FROM recurrence
	// WHERE reminder_id = ?
	// `,
	// 	query.RecurrenceGetByWeekday: `
	// SELECT
	//     id,
	//     reminder_id,
	//     offset,
	//     recur_type,
	//     max_count,
	//     counter,
	//     weekdays,
	//     changed,
	//     uuid
	// FROM recurrence
	// WHERE (weekdays & (1 << ?)) <> 0
	// `,
}
