#!/bin/sh


# Make sence of the rcorder system.
# To see what happens when adding/removing rc.d scripts.

RCDIR=/etc/rc.d/

[ -d $1 ] && RCDIR=$1

for I in `rcorder ${RCDIR}/*` ; do
	echo "${I}"
	grep "# REQUIRE:\|# PROVIDE:\|# BEFORE:\|# KEYWORD:" ${I}
	echo ""
done
