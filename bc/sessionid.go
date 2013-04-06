// Copyright (c) 2013 Mathieu Turcotte
// Licensed under the MIT license.

package bc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const bytesPerSessionId = 16

var nullSessionId = SessionId{}

type SessionId struct {
	bytes [bytesPerSessionId]byte
}

func newSesionId() (sid SessionId, err error) {
	sid = SessionId{}
	n, err := rand.Read(sid.bytes[:])
	if n != bytesPerSessionId && err == nil {
		err = fmt.Errorf("unable to generate session id (read %d bytes "+
			"but needed %d)", n, bytesPerSessionId)
	}
	return
}

func parseSessionId(repr string) (sid SessionId, err error) {
	sid = nullSessionId

	if len(repr) == 0 {
		return
	}

	b, err := hex.DecodeString(repr)
	if err == nil {
		copy(sid.bytes[:], b)
	}

	return
}

func (s SessionId) String() string {
	return hex.EncodeToString(s.bytes[:])
}
