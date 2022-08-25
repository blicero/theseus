// /home/krylon/go/src/github.com/blicero/theseus/objects/peer.go
// -*- mode: go; coding: utf-8; -*-
// Created on 25. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-25 23:38:12 krylon>

package objects

import "fmt"

//go:generate ffjson peer.go

// Peer represents another instance of the program running on the
// local network, that we can synchronize our state with.
type Peer struct {
	Instance string
	Hostname string
	Domain   string
	Port     int
}

func (p *Peer) String() string {
	return fmt.Sprintf("Peer{ Instance: %q, Hostname: %q, Domain: %q, Port: %d }",
		p.Instance,
		p.Hostname,
		p.Domain,
		p.Port)
} // func (p *Peer) String() string
