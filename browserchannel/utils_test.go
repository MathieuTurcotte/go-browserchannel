// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"net/url"
	"reflect"
	"testing"
)

// TODO: Add cases with port number.
func TestOriginMatcher(t *testing.T) {
	cases := []struct {
		origin   string
		expected bool
	}{
		{"http://1.bc.duplika.ca", true},
		{"https://bc.duplika.ca", true},
		{"https://duplika.ca", true},
		{"http://duplika.ca", true},
		{"http://plika.ca", false},
		{"http://.duplika.ca", false},
		{"http://duplika", false},
		{"duplika.ca", false},
	}

	matcher := makeOriginMatcher("duplika.ca")

	for _, c := range cases {
		m := matcher.MatchString(c.origin)
		if m != c.expected {
			t.Errorf("expected %v, got %v for %s", c.expected, m, c.origin)
		}
	}
}

func TestParseIncomingMaps(t *testing.T) {
	cases := []struct {
		qs     string
		offset int
		maps   []Map
		err    error
	}{
		// Empty request body.
		{"", 0, []Map{}, nil},
		// Request body with a count of 0 maps.
		{"count=0", 0, []Map{}, nil},
		// Request body with a map.
		{"count=1&ofs=0&req0_timestamp=1364151246289&req0_id=0",
			0, []Map{{"timestamp": "1364151246289", "id": "0"}}, nil},
		// Request body with two maps.
		{"count=2&ofs=10&req0_key1=foo&req1_key2=bar",
			10, []Map{{"key1": "foo"}, {"key2": "bar"}}, nil},
		// Request body with invalid request id (req2 should be req1).
		{"count=2&ofs=10&req0_key=val&req3_key=val", 0, nil, errBadMap},
		// Request body with an invalid offset value.
		{"count=1&ofs=abc&req0_key=val", 0, nil, errBadMap},
		// Request body with an invalid key id.
		{"count=1&ofs=abc&reqABC_key=val", 0, nil, errBadMap},
	}

	for i, c := range cases {
		values, _ := url.ParseQuery(c.qs)
		offset, maps, err := parseIncomingMaps(values)
		if err != c.err {
			t.Errorf("case %d: expected error %v, got %s", i, c.err, err)
		}
		if offset != c.offset {
			t.Errorf("case %d: expected offset %v, got %v", i, c.offset, offset)
		}
		if !reflect.DeepEqual(maps, c.maps) {
			t.Errorf("case %d: expected maps %#v, got %#v", i, c.maps, maps)
		}
	}
}
