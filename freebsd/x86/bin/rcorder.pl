#!/usr/bin/perl -w

use strict;

# rcorder.pl (rc-order)
# a tool to help visualize the dependancies, and ordering on BSD rcNG style init scripts.


# We need an ordered array of strings to store the raw output of /sbin/rcorder.
my @rcorder_array = ();


#while(`/sbin/rcorder /etc/rc.d/* 2>/dev/null`) {
#	print "line = $_\n";
#}


`/sbin/rcorder /etc/rc.d/\*`