#!/bin/sh

# a script to start thttpd at startup time.
# by Jon Disnard

MINI_HTTPD=${THTTPD:-"/usr/local/sbin/mini_httpd"}
WEB_ROOT=${WEB_ROOT:-"/usr/local/www"}
PORT=${PORT:-"9090"}
PIDFILE=${PIDFILE:-"/var/run/`basename $0`.pid"}
LOGFILE=${LOGFILE:-"/var/log/`basename $0`.log"}
CGIPAT=${CGIPAT:-"**.cgi"}

case $1 in 
	'start')
		[ -f ${PIDFILE} ] || touch ${PIDFILE}
		[ -f ${LOGFILE} ] || touch ${LOGFILE}
		${MINI_HTTPD} -d "${WEB_ROOT}" -p "${PORT}" -i "${PIDFILE}" -l "${LOGFILE}" -c "${CGIPAT}"
	;;
	'stop')
		kill `cat ${PIDFILE}`
	;;
	'restart')
		kill -1 `cat ${PIDFILE}`
	;;
	*)
		echo "USAGE: $0 start|stop|restart"
		exit 1
	;;
esac

#the end
exit 0
