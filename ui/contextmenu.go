// /home/krylon/go/src/github.com/blicero/theseus/ui/contextmenu.go
// -*- mode: go; coding: utf-8; -*-
// Created on 28. 09. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-28 18:50:31 krylon>

package ui

import (
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
} // func (g *GUI) handleReminderClick(view *gtk.TreeView, evt *gdk.Event)
