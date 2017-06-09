/* $Id: asrbl.c,v 1.48 2002/03/05 10:34:09 eriang Exp $ */

/*
 * File:    mini_init.c
 * By:      Erik Anggard, PacketFront Sweden AB
 * Created: 2001-10-02
 *
 * Minimal init process.
 *
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <fcntl.h>
#include <unistd.h>
#include <signal.h>
#include <sys/ioctl.h>
#include <sys/param.h>
#include <sys/reboot.h>
#include <sys/mount.h>
#include <ufs/ufs/ufsmount.h>
#include <sys/cdefs.h>
#include <sys/socket.h>
#include <stddef.h>
#include <syslog.h>



/*
 * Defines
 */
#define DEFAULT_ROOTUID -2
#define PATH_CONSOLE "/dev/console"
#define MAXCMD 128


/*
 * Prototypes
 */
static void reboot_system(void);
static void critical(char *msg);
static int setup_tty(char *name, int redirect);
void badsig_handler(int sig);
static int setup_signals(void);


/*
 * Globals 
*/
int single = 0;


/*
 * main
 */
int 
main(int argc, char *argv[])
{
	struct ufs_args args;
	char cmdstr[MAXCMD+1], *s, *s2;
	int done;

	if (argc == 2 && !strcmp(argv[1], "-s"))
		single = 1;
		
	if (single) {
		openlog("init", LOG_CONS|LOG_ODELAY, LOG_AUTH);

		/* remount root fs in read-write */
		bzero(&args, sizeof(struct ufs_args));
		args.export.ex_root = DEFAULT_ROOTUID;
		if (mount(MOUNT_FFS, "/", MNT_UPDATE, &args) == -1)
			syslog(LOG_ALERT, "init: failed to remount root files "
			       "system read-write\n");

		setup_signals();

		setup_tty(PATH_CONSOLE, 1);

	}


	done = 0;
	while (!done) {
		/* Display simple prompt. */
		printf("# ");
		if ((s = fgets(cmdstr, MAXCMD, stdin)) != NULL) {
			/* strip white space */
			while (isspace(*s)) s++;
			s2 = s;
			while (*s2 != '\0') s2++;
			s2--;
			while (isspace(*s2) && s2 > s) s2--;
			s2[1] = '\0';
			if (*s == '\0')
				continue;
			/* Handle commands. */
			if (!strcmp(s, "reboot")) {
				done = 1;
			} else if (!strcmp(s, "test")) {
				printf("test!\n");
			} else {
				printf("unkown command: %s\n", s);
			}
			
		}
	}
	

	if (single)
		reboot_system();

	return (0);
}


/*
 * reboot
 */
static void 
reboot_system(void)
{

	if (reboot(RB_AUTOBOOT, NULL) == -1)
		perror("reboot");

	for (;;);
}


/*
 * critical
 */
static void 
critical(char *msg)
{
	syslog(LOG_EMERG, "%s", msg);
	reboot_system();
}


/* Setup the console for output/input/whatever. Much of this is
 * taken from the NetBSD init and libs. 
 */
static int 
setup_tty(char *name, int redirect)
{
	int fd;

	revoke(name);
	sleep(2); /* NetBSD does this. */

	if((fd = open(name, O_RDWR)) == -1)
		critical("Unable to open tty");

	/* Taken from login_tty */
	setsid();
	if(ioctl(fd, TIOCSCTTY, NULL) == -1)
		critical("Unable to set controlling tty");
	if (redirect) {
		dup2(fd, 0);
		dup2(fd, 1);
		dup2(fd, 2);
	}
	if (fd > 2)
		close(fd);

	return 0;
}


/*
 * badsig_handler
 */
void 
badsig_handler(int sig)
{
	char text[64];

	snprintf(text, 64, "Received signal %d", sig);
	critical(text);
}


/* Signal choices taken from NetBSD init. */
static int 
setup_signals(void)
{
	struct sigaction sa;
	sigset_t mask;
  
	sigemptyset(&sa.sa_mask);
	sa.sa_flags = 0;
	sa.sa_handler = badsig_handler;
	sigaction(SIGSYS, &sa, NULL);
	sigaction(SIGABRT, &sa, NULL);
	sigaction(SIGFPE, &sa, NULL);
	sigaction(SIGILL, &sa, NULL);
	sigaction(SIGSEGV, &sa, NULL);
	sigaction(SIGBUS, &sa, NULL);
	sigaction(SIGXCPU, &sa, NULL);
	sigaction(SIGXFSZ, &sa, NULL);
	sigaction(SIGTERM, &sa, NULL);
	
	sigemptyset(&sa.sa_mask);
	sa.sa_flags = 0;
	sa.sa_handler = (void *)reboot_system;
	sigaction(SIGUSR1, &sa, NULL);
	
	sa.sa_flags = SA_NOCLDSTOP | SA_RESTART;
	sa.sa_handler = badsig_handler;
	sigaction(SIGCHLD, &sa, NULL);

	sigfillset(&mask);
	sigdelset(&mask, SIGABRT);
	sigdelset(&mask, SIGFPE);
	sigdelset(&mask, SIGILL);
	sigdelset(&mask, SIGSEGV);
	sigdelset(&mask, SIGBUS);
	sigdelset(&mask, SIGSYS);
	sigdelset(&mask, SIGXCPU);
	sigdelset(&mask, SIGXFSZ);
	sigdelset(&mask, SIGHUP);
	sigdelset(&mask, SIGTERM);
	sigdelset(&mask, SIGUSR1);
	
	sigprocmask(SIG_SETMASK, &mask, NULL);
	sigemptyset(&sa.sa_mask);

	return 0;
}




