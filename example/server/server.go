// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package main

import (
	"flag"
	"fmt"
	"github.com/MathieuTurcotte/go-browserchannel/bc"
	"log"
	"net/http"
	"sync"
)

var public = flag.String("public_directory", "", "path to public directory")
var port = flag.String("port", "8080", "the port to listen on")
var hostname = flag.String("hostname", "hpenvy.local", "the server hostname")

var channels = struct {
	sync.RWMutex
	m map[bc.SessionId]*bc.Channel
}{m: make(map[bc.SessionId]*bc.Channel)}

func broadcast(m *bc.Map) {
	channels.RLock()
	defer channels.RUnlock()

	for _, c := range channels.m {
		c.SendArray([]interface{}{fmt.Sprintf("%#v", *m)})
	}
}

func HandleChannel(channel *bc.Channel) {
	log.Printf("Handlechannel (%q)\n", channel.Sid)

	channels.Lock()
	channels.m[channel.Sid] = channel
	channels.Unlock()

	for {
		m, ok := channel.ReadMap()
		if !ok {
			log.Printf("%s: returned with no data, closing\n", channel.Sid)

			channels.Lock()
			delete(channels.m, channel.Sid)
			channels.Unlock()
			break
		}

		log.Printf("%s: map: %#v\n", channel.Sid, *m)
		broadcast(m)
	}
}

func main() {
	flag.Parse()

	handler := bc.NewHandler()
	handler.SetCrossDomainPrefix(*hostname+":"+*port, []string{"bc0", "bc1"})

	http.Handle("/channel/", handler)
	http.Handle("/", http.FileServer(http.Dir(*public)))

	// TODO: If no one reads from the handler, new channels won't be
	// served and the whole server will block. This is a problem.
	go func() {
		for {
			channel, err := handler.Accept()
			if err != nil {
				log.Fatal("handler.Accept: ", err)
			}
			go HandleChannel(channel)
		}
	}()

	err := http.ListenAndServe(":"+*port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
