#!/usr/bin/env bash
# Mathieu Turcotte (c) 2013

SCRIPTPATH=$(cd $(dirname $0); pwd -P)
COMPILERJAR=$SCRIPTPATH/../compiler.jar
CLOSUREDIR=$SCRIPTPATH/../closure-library
CLIENTDIR=$SCRIPTPATH/client

python $CLOSUREDIR/closure/bin/build/closurebuilder.py \
  --root=$CLOSUREDIR/ \
  --root=$CLIENTDIR/ \
  --namespace="tests.start" \
  --output_mode=compiled \
  --compiler_flags="--compilation_level=ADVANCED_OPTIMIZATIONS" \
  --compiler_flags="--warning_level=VERBOSE" \
  --compiler_flags="--externs=$SCRIPTPATH/externs/phantom.js" \
  --compiler_jar=$COMPILERJAR > /dev/null
