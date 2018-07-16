// CookieJar - A contestant's algorithm toolbox
// Copyright 2014 Peter Szilagyi. All rights reserved.
//
// CookieJar is dual licensed: use of this source code is governed by a BSD
// license that can be found in the LICENSE file. Alternatively, the CookieJar
// toolbox may be used in accordance with the terms and conditions contained
// in a signed written agreement between you and the author(s).

package main

import (
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"gopkg.in/fsnotify.v1"
	"gopkg.in/inconshreveable/log15.v2"
	"gopkg.in/karalabe/cookiejar.v2/tools/deps"
)

// Constants used by the arena backend
const wrapperStart = "/**************\nOriginal source\n"
const wrapperEnd = "\nOriginal source\n**************/\n\n"

// Creates a new arena backend to negotiate code snippets.
func backend() (int, error) {
	// Find an unused port and listen on that
	addr, err := net.ResolveTCPAddr("tcp4", "localhost:33214")
	if err != nil {
		return -1, err
	}
	// Create the file system monitor
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return -1, nil
	}
	watches = make(map[string]*websocket.Conn)
	go monitor()

	// Register the websocket handlers
	http.HandleFunc("/", endpoint)
	go func() {
		log15.Info("Starting backend", "address", addr.String())
		if err := http.ListenAndServe(addr.String(), nil); err != nil {
			log15.Crit("failed to start backend", "error", err)
			os.Exit(-1)
		}
	}()
	return addr.Port, nil
}

// Upgrader to convert a simple HTTP request to a websocket connection.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// Data associated with a challenge.
type challenge struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// Websocket inbound connection handler.
func endpoint(w http.ResponseWriter, r *http.Request) {
	log15.Info("inbound websocket connection")

	// Upgrade the request to a websocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log15.Error("failed to upgrade to ws connection", "error", err)
		return
	}
	defer conn.Close()

	open, conflict := "", ""
	for {
		// Fetch the challenge details
		msg := new(challenge)
		if err := conn.ReadJSON(&msg); err != nil {
			log15.Error("failed to retrieve challenge data", "error", err)
			return
		}
		// Pre process the source code
		msg.Name = strings.TrimSpace(msg.Name)
		if strings.Contains(msg.Source, wrapperStart) && strings.Contains(msg.Source, wrapperEnd) {
			msg.Source = strings.Split(msg.Source, wrapperStart)[1]
			msg.Source = strings.Split(msg.Source, wrapperEnd)[0]
		}
		// If it's a new challenge, add it to the repository
		root := filepath.Join(*repo, msg.Name)
		main := filepath.Join(root, "main.go")

		if _, err := os.Stat(root); err != nil {
			log15.Info("new challenge found", "name", msg.Name)
			if err := os.MkdirAll(root, 0700); err != nil {
				log15.Error("failed to create challenge", "error", err)
				return
			}
			if err := ioutil.WriteFile(main, []byte(msg.Source), 0700); err != nil {
				log15.Error("failed to write challenge", "error", err)
				return
			}
			notify(conn, "success", "New challenge registered: "+msg.Name)
		} else {
			// Otherwise make sure we're not conflicting
			if source, err := ioutil.ReadFile(main); err != nil {
				log15.Error("failed to retrieve existing solution", "error", err)
				return
			} else if string(source) != msg.Source {
				log15.Warn("solution conflict, download denied", "name", msg.Name)
				if conflict == msg.Name {
					notify(conn, "warning", "Solution conflict, download denied: "+msg.Name)
				}
				conflict = msg.Name
				continue
			}
			conflict = ""
			if open != msg.Name {
				open = msg.Name
				notify(conn, "information", "Challenge accepting uploads: "+msg.Name)
			}
		}
		// Try to monitor the file
		if _, ok := watches[msg.Name]; !ok {
			log15.Info("starting challenge monitoring", "name", msg.Name)
			if err := watcher.Add(root); err != nil {
				log15.Error("failed to monitor the challenge", "error", err)
				return
			}
			watches[msg.Name] = conn
		}
	}
}

// File system monitor to detect changes.
var watcher *fsnotify.Watcher
var watches map[string]*websocket.Conn

// Keeps processing monitoring events and reacts to source changes.
func monitor() {
	for {
		select {
		case event := <-watcher.Events:
			if path.Base(event.Name) == "main.go" {
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					dir, _ := path.Split(event.Name)
					name := path.Base(dir)
					log15.Info("uploading modified solution", "name", name)

					// Retrieve the user solution and wrap it in a comment block
					source, err := ioutil.ReadFile(event.Name)
					if err != nil {
						log15.Error("failed to retrieve solution", "error", err)
						continue
					}
					wrapped := wrapperStart + string(source) + wrapperEnd

					// Merge all the dependencies to generate the submission
					merged, err := deps.Merge(event.Name, true)
					if err != nil {
						log15.Error("failed to merge submit dependencies", "errir", err)
						continue
					}
					// Serialize the wrapped original and the standalone submission
					conn := watches[name]
					if err := conn.WriteJSON(&challenge{Name: name, Source: wrapped + string(merged)}); err != nil {
						log15.Error("failed to upload new solution", "error", err)
						continue
					}
					notify(conn, "success", "Submission uploaded: "+name+" at "+time.Now().Format("15:04:05"))
				}
			}
		case err := <-watcher.Errors:
			log15.Error("file system monitor failure", "error", err)
		}
	}
}
