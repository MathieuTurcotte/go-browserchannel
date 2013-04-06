// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

// Package browserchannel provides a server-side browser channel
// implementation. See http://goo.gl/F287G for the client-side API.
package bc

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The browser channel protocol version implemented by this library.
const SupportedProcolVersion = "8"

var (
	ErrClosed       = errors.New("handler closed")
	ErrBadMap       = errors.New("bad map")
	ErrBodyTooLarge = errors.New("body too large")
)

const (
	DefaultBindPath = "bind"
	DefaultTestPath = "test"
)

// Possible values for the query TYPE parameter.
// TODO(mturcotte): The query parameter should be parsed and represented as
// real types internally.
const (
	queryTerminate = "terminate"
	queryXmlHttp   = "xmlhttp"
	queryHtml      = "html"
)

// Set of default headers returned for each XML HTTP requests.
var xmlHttpHeaders = map[string]string{
	"Content-Type":           "text/plain",
	"Cache-Control":          "no-cache, no-store, max-age=0, must-revalidate",
	"Expires":                "Fri, 01 Jan 1990 00:00:00 GMT",
	"X-Content-Type-Options": "nosniff",
	"Transfer-Encoding":      "chunked",
	"Pragma":                 "no-cache",
}

// Headers returned to html requests (IE < 10).
var htmlHeaders = map[string]string{
	"Content-Type":           "text/html",
	"Cache-Control":          "no-cache, no-store, max-age=0, must-revalidate",
	"Expires":                "Fri, 01 Jan 1990 00:00:00 GMT",
	"X-Content-Type-Options": "nosniff",
	"Transfer-Encoding":      "chunked",
	"Pragma":                 "no-cache",
}

func getHeaders(queryType string) (headers *map[string]string) {
	if queryType == queryHtml {
		headers = &htmlHeaders
	} else {
		headers = &xmlHttpHeaders
	}
	return
}

type channelMap struct {
	sync.RWMutex
	m map[SessionId]*Channel
}

func (m *channelMap) get(sid SessionId) *Channel {
	m.RLock()
	defer m.RUnlock()
	return m.m[sid]
}

func (m *channelMap) set(sid SessionId, channel *Channel) {
	m.Lock()
	defer m.Unlock()
	m.m[sid] = channel
}

func (m *channelMap) del(sid SessionId) (deleted bool) {
	m.Lock()
	defer m.Unlock()
	_, deleted = m.m[sid]
	delete(m.m, sid)
	return
}

// Contains the browser channel cross domain info for a single domain.
type crossDomainInfo struct {
	hostMatcher *regexp.Regexp
	domain      string
	prefix      string
}

func getHostPrefix(info *crossDomainInfo) string {
	if info != nil {
		return info.prefix
	}
	return ""
}

type Handler struct {
	corsInfo *crossDomainInfo
	prefix   string
	channels *channelMap
	bindPath string
	testPath string
	gcChan   chan SessionId
	connChan chan *Channel
}

// Creates a new browser channel handler.
func NewHandler() (h *Handler) {
	h = new(Handler)
	h.channels = &channelMap{m: make(map[SessionId]*Channel)}
	h.bindPath = DefaultBindPath
	h.testPath = DefaultTestPath
	h.gcChan = make(chan SessionId, 10)
	h.connChan = make(chan *Channel)
	return
}

// Sets the cross domain information for this browser channel. The origin is
// used as the Access-Control-Allow-Origin header value and should respect the
// format specified in http://www.w3.org/TR/cors/. The prefix is used to set
// the hostPrefix parameter on the client side.
func (h *Handler) SetCrossDomainPrefix(domain string, prefix string) {
	h.corsInfo = &crossDomainInfo{makeOriginMatcher(domain), domain, prefix}
}

// Initializes the browser channel handler.
func (h *Handler) Init() *Handler {
	go h.removeClosedSession()
	return h
}

// Accept waits for and returns the next channel to the listener.
func (h *Handler) Accept() (channel *Channel, err error) {
	channel, ok := <-h.connChan
	if !ok {
		err = ErrClosed
	}
	return
}

// Removes closed channels from the handler's channel map.
func (h *Handler) removeClosedSession() {
	for {
		sid, ok := <-h.gcChan
		if !ok {
			break
		}

		log.Printf("%s: removing from session map\n", sid)

		if !h.channels.del(sid) {
			log.Printf("%s: missing channel from session map\n", sid)
		}
	}
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// The CORS  spec only supports *, null or the exact domain.
	// http://www.w3.org/TR/cors/#access-control-allow-origin-response-header
	// http://tools.ietf.org/html/rfc6454#section-7.1
	origin := req.Header.Get("origin")
	if len(origin) > 0 && h.corsInfo != nil &&
		h.corsInfo.hostMatcher.MatchString(origin) {
		rw.Header().Set("Access-Control-Allow-Origin", origin)
		rw.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// The body is parsed before calling ParseForm so the values don't get
	// collapsed into a single collection.
	values, err := parseBody(req.Body)
	if err != nil {
		rw.WriteHeader(400)
		return
	}

	req.ParseForm()
	path := req.URL.Path
	if strings.HasSuffix(path, h.testPath) {
		h.handleTestRequest(rw, req)
	} else if strings.HasSuffix(path, h.bindPath) {
		h.handleBindRequest(rw, req, values)
	} else {
		rw.WriteHeader(404)
	}
}

func (h *Handler) handleTestRequest(rw http.ResponseWriter, req *http.Request) {
	log.Printf("test:%s\n", req.URL)

	version := req.Form.Get("VER")
	mode := req.Form.Get("MODE")
	qtype := req.Form.Get("TYPE")
	domain := req.Form.Get("DOMAIN")

	if version != SupportedProcolVersion {
		rw.Header().Set("Status", "Unsupported protocol version.")
		rw.WriteHeader(400)
		io.WriteString(rw, "Unsupported protocol version.")
	} else if mode == "init" {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		io.WriteString(rw, "[\""+getHostPrefix(h.corsInfo)+"\",\"\"]")
	} else {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, getHeaders(qtype))
		rw.WriteHeader(200)

		if qtype == queryHtml {
			writeHtmlHead(rw)
			writeHtmlDomain(rw, domain)
			writeHtmlRpc(rw, "11111")
			writeHtmlPadding(rw)
		} else {
			io.WriteString(rw, "11111")
		}

		time.Sleep(2 * time.Second)

		if qtype == queryHtml {
			writeHtmlRpc(rw, "2")
			writeHtmlDone(rw)
		} else {
			io.WriteString(rw, "2")
		}
	}
}

func (h *Handler) handleBindRequest(rw http.ResponseWriter, req *http.Request,
	values url.Values) {
	log.Printf("bind:%s:%s\n", req.Method, req.URL)

	var channel *Channel

	sid, err := parseSessionId(req.Form.Get("SID"))

	if err != nil {
		rw.WriteHeader(400)
		return
	}

	// If the client has specified a session id, lookup the session object
	// in the sessions map. Lookup failure should be signaled to the client
	// using a 400 status code and a message containing 'Unknown SID'. See
	// goog/net/channelrequest.js for more context on how this error is
	// handled.
	if sid != nullSessionId {
		channel = h.channels.get(sid)
		if channel == nil {
			log.Printf("failed to lookup session (%q)\n", sid)
			rw.Header().Set("Status", "Unknown SID")
			setHeaders(rw, &xmlHttpHeaders)
			rw.WriteHeader(400)
			io.WriteString(rw, "Unknown SID")
			return
		}
	}

	if channel == nil {
		sid, _ = newSesionId()
		log.Printf("%s: creating session\n", sid)
		channel = newChannel(req.Form.Get("VER"), sid, h.gcChan, h.corsInfo)
		h.channels.set(sid, channel)
		h.connChan <- channel
		go channel.start()
	}

	if aid, err := strconv.Atoi(req.Form.Get("AID")); err == nil {
		channel.acknowledge(aid)
	}

	switch req.Method {
	case "POST":
		h.handleBindPost(rw, req, channel, values)
	case "GET":
		h.handleBindGet(rw, req, channel)
	default:
		rw.WriteHeader(400)
	}
}

func (h *Handler) handleBindPost(rw http.ResponseWriter, r *http.Request,
	channel *Channel, values url.Values) {
	log.Printf("%s: bind parameters: %v\n", channel.Sid, values)

	offset, maps, err := parseIncomingMaps(values)
	if err != nil {
		rw.WriteHeader(400)
		return
	}

	channel.receiveMaps(offset, maps)

	if channel.state == channelInit {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		rw.(http.Flusher).Flush()

		// The initial forward request is used as a back channel to send the
		// server configuration: ['c', id, host, version]. This payload has to
		// be sent immediately, so the streaming is disabled on the back
		// channel. Note that the first bind request made by IE<10 does not
		// contain a TYPE=html query parameter and therefore receives the same
		// length prefixed array reply as is sent to the XHR streaming clients.
		backChannel := newBackChannel(channel.Sid, rw, false, "", "")
		channel.setBackChannel(backChannel)
		backChannel.wait()
	} else {
		// On normal forward channel request, the session status is returned
		// to the client. The session status contains 3 pieces of information:
		// does this session has a back channel, the last array id sent to the
		// client and the number of outstanding bytes in the back channel.
		b, _ := json.Marshal(channel.getState())
		rw.Header().Set("Status", "OK")
		setHeaders(rw, &xmlHttpHeaders)
		rw.WriteHeader(200)
		io.WriteString(rw, strconv.FormatInt(int64(len(b)), 10)+"\n")
		rw.Write(b)
	}
}

func (h *Handler) handleBindGet(
	rw http.ResponseWriter, req *http.Request, channel *Channel) {
	queryType := req.Form.Get("TYPE")
	domain := req.Form.Get("DOMAIN")
	rid := req.Form.Get("zx")

	if queryType == queryTerminate {
		channel.Close()
	} else {
		rw.Header().Set("Status", "OK")
		setHeaders(rw, getHeaders(queryType))
		rw.WriteHeader(200)
		rw.(http.Flusher).Flush()

		isHtml := queryType == queryHtml
		bc := newBackChannel(channel.Sid, rw, isHtml, domain, rid)
		bc.setChunked(req.Form.Get("CI") == "1")
		channel.setBackChannel(bc)
		bc.wait()
		log.Printf("%s: bind wait done\n", channel.Sid)
	}
}
