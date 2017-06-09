#!/bin/sh

HOME=/root
PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/libexec
TERM=wsvt25
BLOCKSIZE=1k
EDITOR=vi
PAGER=less
export PATH
export TERM
export BLOCKSIZE
export EDITOR
export PAGER
export HOME

# proceed to the custom rc file
#. conf/rc
