// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package bc

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBadMap       = errors.New("bad map")
	ErrBodyTooLarge = errors.New("body too large")
)

// Creates a regexp that matches an origin and all its subdomains. Both http
// and https schemes are accepted.
func makeOriginMatcher(domain string) *regexp.Regexp {
	pattern := `^https?://([[:alnum:]]+\.)*` + regexp.QuoteMeta(domain) + `$`
	return regexp.MustCompile(pattern)
}

func setHeaders(rw http.ResponseWriter, headers *map[string]string) {
	for k, v := range *headers {
		rw.Header().Set(k, v)
	}
}

// Parses the body of a POST request.
func parseBody(r io.ReadCloser) (values url.Values, err error) {
	maxFormSize := int64(10 << 20)

	// Let the reader read 1 more byte and blow up if he does.
	reader := io.LimitReader(r, maxFormSize+1)

	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return
	}

	if int64(len(body)) > maxFormSize {
		err = ErrBodyTooLarge
		return
	}

	values, err = url.ParseQuery(string(body))
	return
}

// Transforms the url values of the following format
//
// {
//     count: '2',
//     ofs: '3',
//     req0_x: '3',
//     req0_y: '10',
//     req1_abc: 'def'
// }
//
// into this
//
// {
//     offset: 3,
//     maps: [
//         {x: '3', y: '10'},
//         {abc: 'def'}
//     ]
// }
func parseIncomingMaps(values url.Values) (offset int, maps []Map, err error) {
	count, _ := strconv.Atoi(values.Get("count"))

	if count == 0 {
		maps = []Map{}
		return
	}

	offset, err = strconv.Atoi(values.Get("ofs"))

	if err != nil {
		offset = 0
		err = ErrBadMap
		return
	}

	// TODO: Limit number of maps to avoid DOS attack.
	maps = make([]Map, count)
	for i := range maps {
		maps[i] = make(Map)
	}

	for key, values := range values {
		err = parseMapEntry(maps, count, key, values[0])
		if err != nil {
			offset = 0
			maps = nil
			return
		}
	}

	return
}

func parseMapEntry(maps []Map, count int, key string, value string) (err error) {
	// TODO: Figure out the correct way to handle that case.
	if value == "_badmap" {
		return
	}

	// Split "req9_key" in two parts ["9", "key"].
	keyParts := strings.Split(strings.TrimLeft(key, "req"), "_")

	if len(keyParts) == 2 {
		id, cerr := strconv.Atoi(keyParts[0])
		if cerr != nil || id > count {
			err = ErrBadMap
			return
		}
		mapKey := keyParts[1]
		maps[id][mapKey] = value
	}

	return
}

// Helper that returns a channel of time if the time.Timer is defined and and a
// nil channel otherwise.
func timerChan(t *time.Timer) (c <-chan time.Time) {
	if t != nil {
		c = t.C
	}
	return
}

// Helper that returns a channel of time if the time.Ticker is defined and and
// a nil channel otherwise.
func tickerChan(t *time.Ticker) (c <-chan time.Time) {
	if t != nil {
		c = t.C
	}
	return
}
