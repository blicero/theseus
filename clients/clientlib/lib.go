// /home/krylon/go/src/github.com/blicero/theseus/clients/clientlib/lib.go
// -*- mode: go; coding: utf-8; -*-
// Created on 14. 08. 2022 by Benjamin Walkenhorst
// (c) 2022 Benjamin Walkenhorst
// Time-stamp: <2022-08-16 19:19:08 krylon>

// Package clientlib provides the basic framework for
// building clients that create new Notifications.
package clientlib

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/blicero/theseus/common"
	"github.com/blicero/theseus/logdomain"
	"github.com/blicero/theseus/objects"
	"github.com/pquerna/ffjson/ffjson"
)

const (
	addPath = "/reminder/add"
)

// Client is the basic implementation of a Theseus client,
// it implements the fundamental communication with the Server.
type Client struct {
	Server *url.URL
	Client http.Client
	log    *log.Logger
}

// NewClient creates a new Client.
func NewClient(srv string) (*Client, error) {
	var (
		err error
		c   = &Client{
			Client: http.Client{
				Timeout: time.Second * 10,
			},
		}
	)

	if c.log, err = common.GetLogger(logdomain.Client); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"Cannot create Logger: %s\n",
			err.Error())
		return nil, err
	} else if c.Server, err = url.Parse(srv); err != nil {
		c.log.Printf("[ERROR] Cannot parse URL %q: %s\n",
			srv,
			err.Error())
		return nil, err
	}

	c.Server.Scheme = "http"
	c.Server.Path = addPath

	return c, nil
} // func NewClient(srv string) (*Client, error)

func (c *Client) GetLogger() *log.Logger {
	return c.log
} // func (c *Client) GetLogger() *log.Logger

func (c *Client) SubmitReminder(r *objects.Reminder) error {
	var (
		err     error
		sendBuf []byte
		msg     string
		rcvBuf  bytes.Buffer
		hres    *http.Response
		ores    objects.Response
		values  = make(url.Values)
	)

	if sendBuf, err = ffjson.Marshal(r); err != nil {
		c.log.Printf("[ERROR] Cannot serizalize Reminder: %s\n",
			err.Error())
		return err
	}

	defer ffjson.Pool(sendBuf)

	values["reminder"] = []string{string(sendBuf)}

	if hres, err = c.Client.PostForm(c.Server.String(), values); err != nil {
		c.log.Printf("[ERROR] Failed to POST Reminder to %s: %s\n",
			c.Server,
			err.Error())
		return err
	} else if hres.StatusCode != http.StatusOK {
		msg = fmt.Sprintf("Unexpected status from %s: %s",
			c.Server,
			hres.Status)
		c.log.Printf("[ERROR] %s\n", msg)
		return errors.New(msg)
	} else if _, err = io.Copy(&rcvBuf, hres.Body); err != nil {
		c.log.Printf("[ERROR] Failed to read Response body from %s: %s\n",
			c.Server,
			err.Error())
		return err
	} else if err = ffjson.Unmarshal(rcvBuf.Bytes(), &ores); err != nil {
		c.log.Printf("[ERROR] Cannot de-serialize Response from %s: %s\n",
			c.Server,
			err.Error())
		return err
	} else if !ores.Status {
		err = fmt.Errorf("Request to %s failed: %s",
			c.Server,
			ores.Message)
		c.log.Printf("[ERROR] %s\n",
			err.Error())
		return err
	}

	c.log.Printf("[DEBUG] Request to %s was successful: %s\n",
		c.Server,
		ores.Message)

	return nil
} // func (c *Client) SubmitReminder(r *objects.Reminder) error
