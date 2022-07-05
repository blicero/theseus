// /home/krylon/go/src/github.com/blicero/theseus/objects/response.go
// -*- mode: go; coding: utf-8; -*-
// Created on 05. 07. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-07-05 20:11:13 krylon>

package objects

//go:generate ffjson response.go

// Response is what the backend sends to a client after processing a request.
type Response struct {
	ID      int64
	Status  bool
	Message string
}
