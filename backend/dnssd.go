// /home/krylon/go/src/github.com/blicero/theseus/backend/dnssd.go
// -*- mode: go; coding: utf-8; -*-
// Created on 24. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-09-15 19:15:17 krylon>

package backend

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/grandcat/zeroconf"
)

const (
	srvName    = "Theseus"
	srvService = "_http._tcp"
	srvDomain  = "local."
	srvTTL     = 5
)

var (
	addrPat = regexp.MustCompile(`:(\d+)$`)
)

func (d *Daemon) initDNSSd() error {
	var (
		err   error
		match []string
		port  int64
		srv   *zeroconf.Server
	)

	match = addrPat.FindStringSubmatch(d.web.Addr)

	if port, err = strconv.ParseInt(match[1], 10, 16); err != nil {
		d.log.Printf("[ERROR] Cannot parse HTTP port from server address %q: %s\n",
			d.web.Addr,
			err.Error())
		return err
	}

	var txt = []string{"txtv=0", "lo=1", "la=2"}

	var instanceName = fmt.Sprintf("%s@%s",
		srvName,
		d.hostname)

	if srv, err = zeroconf.Register(instanceName, srvService, srvDomain, int(port), txt, nil); err != nil {
		d.log.Printf("[ERROR] Cannot register service with DNS-SD: %s\n",
			err.Error())
		return err
	}

	// Unfortunately, this triggers a race condition
	// srv.TTL(srvTTL)

	d.dnssd = srv
	return nil
} // func (d *Daemon) initDnsSd() error

func (d *Daemon) findPeers() {

	go d.purgeLoop()

	for d.IsAlive() {
		var (
			err      error
			resolver *zeroconf.Resolver
			entries  chan *zeroconf.ServiceEntry
		)

		if resolver, err = zeroconf.NewResolver(nil); err != nil {
			d.log.Printf("[ERROR] Cannot create DNS-SD Resolver: %s\n",
				err.Error())
			return
		}

		entries = make(chan *zeroconf.ServiceEntry)

		go d.processServiceEntries(entries)

		ctx, cancel := context.WithCancel(context.Background())
		// defer cancel()

		if err = resolver.Browse(ctx, srvService, srvDomain, entries); err != nil {
			d.log.Printf("{ERROR] Failed to browse for %s: %s\n",
				srvService,
				err.Error())
		}

		time.Sleep(time.Second * srvTTL)
		cancel()

	}
} // func (d *Daemon) findPeers()

func (d *Daemon) processServiceEntries(queue <-chan *zeroconf.ServiceEntry) {
	defer d.log.Println("[INFO] DNS-SD Listener is quitting.")
	var peerPat = regexp.MustCompile(fmt.Sprintf("%s\\\\@(\\w+)", common.AppName))

	for entry := range queue {
		var str = rrStr(entry)
		// d.log.Printf("[DEBUG] Received one ServiceEntry: %s\n",
		// 	str)

		if strings.HasPrefix(entry.HostName, d.hostname) {
			continue
		} else if !peerPat.MatchString(entry.Instance) {
			continue
		}

		entry.TTL = srvTTL

		// d.log.Println("[TRACE] Acquire pLock")
		d.pLock.Lock()
		d.peers[str] = mkService(entry)
		d.pLock.Unlock()
		// d.log.Println("[TRACE] Released pLock")
	}
} // func (d *Daemon) processServiceEntries(queue <- chan *zeroconf.ServiceEntry)

func (d *Daemon) purgeLoop() {
	for d.IsAlive() {
		time.Sleep(time.Second * srvTTL)

		d.pLock.Lock()
		for k, srv := range d.peers {
			if srv.isExpired() {
				d.log.Printf("[DEBUG] Remove Peer %s from cache\n",
					k)
				delete(d.peers, k)
			}
		}
		d.pLock.Unlock()
	}
} // func (d *Daemon) purgeLoop()
