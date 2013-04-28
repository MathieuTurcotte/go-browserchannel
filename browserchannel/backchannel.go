// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"io"
	"log"
	"net/http"
	"strconv"
)

// The back channel interface shared between the XHR and HTML implementations.
type backChannel interface {
	getRequestId() string
	isReusable() bool
	setChunked(bool)
	isChunked() bool
	send(data []byte) (int, error)
	discard()
	wait()
}

// Common bookeeping information shared between the chunked XHR and HTML
// variants.
type backChannelBase struct {
	sid       SessionId
	rid       string
	w         http.ResponseWriter
	chunked   bool
	bytesSent int
	done      chan bool
}

func (b *backChannelBase) getRequestId() string {
	return b.rid
}

func (b *backChannelBase) isReusable() bool {
	return b.chunked && b.bytesSent < 10*1024
}

func (b *backChannelBase) setChunked(chunked bool) {
	b.chunked = chunked
}

func (b *backChannelBase) isChunked() bool {
	return b.chunked
}

func (b *backChannelBase) wait() {
	<-b.done
	log.Printf("%s[%s] bind wait done\n", b.sid, b.rid)
}

// The chunked XHR back channel implementation.
type xhrBackChannel struct {
	backChannelBase
}

func (b *xhrBackChannel) send(data []byte) (n int, err error) {
	log.Printf("%s[%s] xhr back channel send: %s\n", b.sid, b.rid, data)

	n, err = io.WriteString(b.w,
		strconv.FormatInt(int64(len(data)), 10)+"\n"+string(data))

	b.w.(http.Flusher).Flush()

	if err == nil {
		b.bytesSent += n
	}

	return
}

func (b *xhrBackChannel) discard() {
	log.Printf("%s[%s] xhr back channel close\n", b.sid, b.rid)
	close(b.done)
}

// The chunked HTML back channel implementation used for IE9 and older.
type htmlBackChannel struct {
	backChannelBase
	paddingSent bool
	domain      string
}

func (b *htmlBackChannel) send(data []byte) (n int, err error) {
	log.Printf("%s[%s] html back channel send: %s\n", b.sid, b.rid, data)

	if !b.paddingSent {
		writeHtmlHead(b.w)
		writeHtmlDomain(b.w, b.domain)
	}

	n, err = writeHtmlRpc(b.w, string(data))

	if !b.paddingSent {
		writeHtmlPadding(b.w)
		b.paddingSent = true
	}

	b.w.(http.Flusher).Flush()

	if err == nil {
		b.bytesSent += n
	}

	return
}

func (b *htmlBackChannel) discard() {
	log.Printf("%s %s html back channel close\n", b.sid, b.rid)
	writeHtmlDone(b.w)
	close(b.done)
}

func newBackChannel(sid SessionId, w http.ResponseWriter, html bool,
	domain string, rid string) (bc backChannel) {
	base := backChannelBase{sid: sid, rid: rid, w: w, done: make(chan bool)}

	if html {
		bc = &htmlBackChannel{backChannelBase: base, domain: domain}
	} else {
		bc = &xhrBackChannel{base}
	}
	return
}
