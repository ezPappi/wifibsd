/* Custom /sbin/init for diskless NetBSD test system */

// $Id: init.c,v 1.2 2001/02/13 19:14:50 root Exp $

// $Log: init.c,v $
// Revision 1.2  2001/02/13 19:14:50  root
// Fixed open comment.
//
// Revision 1.1  2001/02/13 18:24:51  root
// Initial revision

#include <errno.h>
#include <fcntl.h>
#include <syslog.h>
#include <unistd.h>
#include <util.h>

#define CONSOLE "/dev/console"

void errorexit(char *proc)
{
  syslog(LOG_ERR, "ERROR: %s() returned %s", proc, sys_errlist[errno]);
  exit(1);
}

void main(int argc, char *argv[])
{
  int status;
  int f;

  openlog("init", LOG_CONS|LOG_ODELAY, LOG_AUTH);

  status = setsid();
  if (status < 0) errorexit("setsid");

  status = setlogin("root");
  if (status < 0) errorexit("setlogin");

/* Open stdin, stdout, stderr */

  status = revoke(CONSOLE);
  if (status < 0) errorexit("revoke");

  f = open(CONSOLE, O_RDWR, 0);
  if (f < 0) errorexit("open");

//  status = login_tty(f);
//  if (status < 0) errorexit("login_tty");

/* Start a command shell */

  status = execl("/bin/sh", "-init");
  if (status < 0) errorexit("exec");
}
