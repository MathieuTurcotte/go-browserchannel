// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package main

import (
	"flag"
	"fmt"
	bc "github.com/MathieuTurcotte/go-browserchannel/browserchannel"
	"log"
	"net/http"
	"sync"
)

var publicDir = flag.String("public_directory", "", "path to public directory")
var closureDir = flag.String("closure_directory", "", "path to closure directory")
var port = flag.String("port", "8080", "the port to listen on")
var hostname = flag.String("hostname", "hpenvy.local", "the server hostname")

var channels = struct {
	sync.RWMutex
	m map[bc.SessionId]*bc.Channel
}{m: make(map[bc.SessionId]*bc.Channel)}

func broadcast(m bc.Map) {
	channels.RLock()
	defer channels.RUnlock()

	for _, c := range channels.m {
		c.SendArray(bc.Array{fmt.Sprintf("%#v", m)})
	}
}

func handleChannel(channel *bc.Channel) {
	log.Printf("Handlechannel (%q)\n", channel.Sid)

	channels.Lock()
	channels.m[channel.Sid] = channel
	channels.Unlock()

	for {
		m, ok := <-channel.Maps()
		if !ok {
			log.Printf("%s: returned with no data, closing\n", channel.Sid)

			channels.Lock()
			delete(channels.m, channel.Sid)
			channels.Unlock()
			break
		}

		log.Printf("%s: map: %#v\n", channel.Sid, m)
		broadcast(m)
	}
}

func main() {
	flag.Parse()

	handler := bc.NewHandler(handleChannel)
	handler.SetCrossDomainPrefix(*hostname+":"+*port, []string{"bc0", "bc1"})

	http.Handle("/channel/", handler)
	http.Handle("/closure/", http.StripPrefix("/closure/", http.FileServer(http.Dir(*closureDir))))
	http.Handle("/", http.FileServer(http.Dir(*publicDir)))

	err := http.ListenAndServe(":"+*port, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
