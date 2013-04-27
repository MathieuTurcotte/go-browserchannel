#!/usr/bin/env bash

set -x

go install github.com/MathieuTurcotte/go-browserchannel/... || exit

bin/server --public_directory=src/github.com/MathieuTurcotte/go-browserchannel/example/client --port=8080 $@
