#!/usr/bin/env bash
# Mathieu Turcotte (c) 2013

set -x

go install github.com/MathieuTurcotte/go-browserchannel/... || exit

GOMAXPROCS=8

bin/server \
    --public_directory=src/github.com/MathieuTurcotte/go-browserchannel/test/client \
    --closure_directory=src/github.com/MathieuTurcotte/go-browserchannel/closure-library \
    --port=8080 $@
