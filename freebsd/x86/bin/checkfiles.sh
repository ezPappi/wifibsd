#!/bin/sh


#checkfiles.sh
# a script to automate the build process of WifiBSD.
# written by Jon Disnard <diz@linuxpowered.com>

# BEST VIEWED WITH TABSTOP=4


# 12/13/2004
# for the moment this script expects to be executed from the dir that contains the wifibsd.files,
# the kernel config, and other stuff.
# this needs to be fixed so the script bails-out when this is not the case.


# SET DEFAULT VARS
EXECPATH=${EXECPATH:-`dirname $(realpath $0)`}          # the is the location of this script.
PWD_PATH=`pwd`
BASEDIR=${BASEDIR:-"/"}                                                         # The place all files are copied from.
DESTDIR=${DESTDIR:-"${PWD_PATH}"}                                       # Main folder everything is default to.
DIRSKEL=${DIRSKEL:-"${DESTDIR}/wifibsd.dirs"}		# The file which contains the list of directories.
FILELIST=${FILELIST:-"${DESTDIR}/wifibsd.files"}	# The file which contains the list of files.
VERBOSE=${VERBOSE:-"0"}								# The default verbosity level.

# include the sub-routines.
. ${EXECPATH}/wifibsd.subr || exit 1


# INSERT SANITY CHECKS HERE
# aka anything that doesn't exist, or not look right; bail.


# You can override any variable with your own configuration file.
[ -f ${DESTDIR}/wifibsd.conf ] && . ${DESTDIR}/wifibsd.conf


[ ${VERBOSE} -gt 0 ] && VERBOPTS="-v"


#
# The main entry point of the script.
#


# make sure we are in correct location.
# When this script is called from jexec(1), the $PWD is always / (root) of the jail.
# This ensures that all the paths work correctly when executed in a jail, from outside the jail.
cd ${EXECPATH}


# the main loop
for LINE in `grep -v "^#" ${FILELIST} | \
grep -v "^$" | \
cut -d "#" -f 1`; do
	set `echo ${LINE} | tr ":" " "`
	while : ; do
		BASEFILE=$1 && shift
		# We check for dir's first.
		[ -e ${SRCPATH}/${BASEFILE} ] || echo "${SRCPATH}/${BASEFILE} does not exist"
		[ $# -eq "0" ] && break
	done
done


info "DONE."
