#!/bin/sh

HOME=/
PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/libexec:/
TERM=wsvt25
BLOCKSIZE=1k
export PATH
export TERM
export BLOCKSIZE
export HOME
umask 022

#Pull in some functions that poeple can use from shell prompt
#dmesg () cat /kern/msgbuf
#grep () sed -n "/$1/p"
#
#if [ -x /etc/rc.start ] ; then
#	# Run the installation script
#	/bin/sh /etc/rc.start
#else
#	echo ""
#	echo "Installation script is MISSING!"
#	echo "Rebooting..."
#	echo ""
#	/sbin/shutdown -r now
#fi
