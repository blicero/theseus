// /home/krylon/go/src/github.com/blicero/theseus/ui/contextmenu.go
// -*- mode: go; coding: utf-8; -*-
// Created on 28. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-28 20:46:40 krylon>

package ui

import (
	"fmt"

	"github.com/blicero/theseus/objects"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func (g *GUI) handleReminderClick(view *gtk.TreeView, evt *gdk.Event) {
	var be = gdk.EventButtonNewFromEvent(evt)

	if be.Button() != gdk.BUTTON_SECONDARY {
		return
	}

	var (
		err    error
		msg    string
		exists bool
		x, y   float64
		path   *gtk.TreePath
		col    *gtk.TreeViewColumn
		model  *gtk.TreeModel
		imodel gtk.ITreeModel
		iter   *gtk.TreeIter
	)

	x = be.X()
	y = be.Y()

	path, col, _, _, exists = view.GetPathAtPos(int(x), int(y))

	if !exists {
		g.log.Printf("[DEBUG] There is no item at %f/%f\n",
			x,
			y)
		return
	}

	g.log.Printf("[DEBUG] Handle Click at %f/%f -> Path %s\n",
		x,
		y,
		path)

	if imodel, err = view.GetModel(); err != nil {
		g.log.Printf("[ERROR] Cannot get Model from View: %s\n",
			err.Error())
		return
	}

	model = imodel.ToTreeModel()

	if iter, err = model.GetIter(path); err != nil {
		g.log.Printf("[ERROR] Cannot get Iter from TreePath %s: %s\n",
			path,
			err.Error())
		return
	}

	var title string = col.GetTitle()
	g.log.Printf("[DEBUG] Column %s was clicked\n",
		title)

	var (
		val *glib.Value
		gv  interface{}
		id  int64
	)

	if val, err = model.GetValue(iter, 0); err != nil {
		g.log.Printf("[ERROR] Cannot get value for column 0: %s\n",
			err.Error())
		return
	} else if gv, err = val.GoValue(); err != nil {
		g.log.Printf("[ERROR] Cannot get Go value from GLib value: %s\n",
			err.Error())
	}

	switch v := gv.(type) {
	case int:
		id = int64(v)
	case int64:
		id = v
	default:
		g.log.Printf("[ERROR] Unexpected type for ID column: %T\n",
			v)
	}

	g.log.Printf("[DEBUG] ID of clicked-on row is %d\n",
		id)

	var (
		r    objects.Reminder
		menu *gtk.Menu
	)

	if r, exists = g.reminders[id]; !exists {
		msg = fmt.Sprintf("Cannot find Reminder #%d",
			id)
		g.log.Printf("[ERROR] %s\n",
			msg)
		g.pushMsg(msg)
		return
	} else if menu, err = g.mkReminderContextMenu(path, &r); err != nil {
		msg = fmt.Sprintf("Cannot create context menu for Reminder %q: %s",
			r.Title,
			err.Error())
		g.log.Printf("[ERROR] %s\n",
			msg)
		g.pushMsg(msg)
		return
	}

	menu.ShowAll()
	menu.PopupAtPointer(evt)
} // func (g *GUI) handleReminderClick(view *gtk.TreeView, evt *gdk.Event)

func (g *GUI) mkReminderContextMenu(path *gtk.TreePath, r *objects.Reminder) (*gtk.Menu, error) {
	var (
		err               error
		msg               string
		editItem, delItem *gtk.MenuItem
		toggleItem        *gtk.CheckMenuItem
		menu              *gtk.Menu
	)

	if menu, err = gtk.MenuNew(); err != nil {
		msg = fmt.Sprintf("Cannot create Menu: %s",
			err.Error())
		goto ERROR
	} else if editItem, err = gtk.MenuItemNewWithMnemonic("_Edit"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Edit: %s",
			err.Error())
		goto ERROR
	} else if delItem, err = gtk.MenuItemNewWithMnemonic("_Delete"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Delete: %s",
			err.Error())
		goto ERROR
	} else if toggleItem, err = gtk.CheckMenuItemNewWithLabel("_Active"); err != nil {
		msg = fmt.Sprintf("Cannot create menu item Active: %s",
			err.Error())
		goto ERROR
	}

	// Set up signal handlers!

	menu.Append(editItem)
	menu.Append(delItem)
	menu.Append(toggleItem)

	return menu, nil
ERROR:
	g.log.Printf("[ERROR] %s\n", msg)
	g.pushMsg(msg)
	g.displayMsg(msg)
	return nil, err
} // func (g *GUI) mkReminderContextMenu(path *gtk.TreePath, r *objects.Reminder) (*gtk.Menu, error)

// func (g *GUI) handleReminderClickEdit() {
// 	var (
// 		err error
// 		msg string

// 	)
// }
// func (g *GUI) handleReminderClickEdit()
