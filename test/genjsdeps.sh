#!/usr/bin/env bash
# Mathieu Turcotte (c) 2013

SCRIPTPATH=$(cd $(dirname $0); pwd -P)
CLOSUREDIR=$SCRIPTPATH/../closure-library
CLIENTDIR=$SCRIPTPATH/client

python $CLOSUREDIR/closure/bin/build/depswriter.py \
    --root_with_prefix="$CLIENTDIR ../../../" > $CLIENTDIR/deps.js
    # ../../../ is the path from base.js to the root of the client files.
