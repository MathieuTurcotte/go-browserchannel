// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package browserchannel

import (
	"bytes"
	"testing"
)

func TestParseSessionId(t *testing.T) {
	cases := []struct {
		str string
		sid []byte
		err error
	}{
		{
			"",
			nullSessionId[:],
			nil,
		}, {
			"b007b243d7054b46",
			nullSessionId[:],
			errInvalidSessionId,
		}, {
			"b007b243d7054b46cab92Zcfa6c0a3b2",
			nullSessionId[:],
			errInvalidSessionId,
		}, {
			"b007b243d7054b46cab926cfa6c0a3b2",
			[]byte{
				0xb0, 0x07, 0xb2, 0x43, 0xd7, 0x05, 0x4b, 0x46,
				0xca, 0xb9, 0x26, 0xcf, 0xa6, 0xc0, 0xa3, 0xb2,
			}, nil,
		},
	}

	for _, c := range cases {
		sid, err := parseSessionId(c.str)
		if err != c.err {
			t.Errorf("expected %v as error, got %v for %s", c.err, err, c.str)
		}
		if !bytes.Equal(sid[:], c.sid) {
			t.Errorf("expected %x as sid, got %v", c.sid, sid)
		}
	}
}
