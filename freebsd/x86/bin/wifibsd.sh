#!/bin/sh


#wifibsd.sh
# a script to automate the build process of WifiBSD.
# written by Jon Disnard <diz@linuxpowered.com>

# BEST VIEWED WITH TABSTOP=4


# 9/8/2003
# for the moment this script expects to be executed from the dir that contains the wifibsd.files,
# the kernel config, and other stuff.
# this needs to be fixed so the script bails-out when this is not the case.


# SET DEFAULT VARS
EXECPATH=${EXECPATH:-`dirname $(realpath $0)`}		# the is the location of this script.
PWD_PATH=`pwd`
ARCH=`uname -p`										# The the default architecture.
BASEDIR=${BASEDIR:-"/"}								# The place all files are copied from.
DESTDIR=${DESTDIR:-"${PWD_PATH}"}					# Main folder everything is default to.
WORKDIR=${WORKDIR:-"${DESTDIR}/work"}				# This is the main work/staging area.
BOOTDEST=${BOOTDEST:-"${DESTDIR}/boot"}				# The /boot and /rescue dirs live here (kern.flp).
ROOTDEST=${ROOTDEST:-"${DESTDIR}/root"}				# Standard root file system (mfsroot.flp).
KERNCONF=${KERNCONF:-"${DESTDIR}/WIFIBSD"}			# The default kernel conf file.
DIRSKEL=${DIRSKEL:-"${DESTDIR}/wifibsd.dirs"}		# The file which contains the list of directories.
FILELIST=${FILELIST:-"${DESTDIR}/wifibsd.files"}	# The file which contains the list of files.
VERBOSE=${VERBOSE:-"0"}								# The default verbosity level.
SSHKEYGEN=${SSHKEYGEN=/usr/bin/ssh-keygen}
LDDIRS="bin sbin libexec usr/bin usr/sbin usr/libexec"
#ROOTPASS=${ROOTPASS:-""}							# The Root (uid 0) password is default to blank.

# include the sub-routines.
. ${EXECPATH}/wifibsd.subr || exit 1


# INSERT SANITY CHECKS HERE
# aka anything that doesn't exist, or not look right; bail.


# You can override any variable with your own configuration file.
[ -f ${DESTDIR}/wifibsd.conf ] && . ${DESTDIR}/wifibsd.conf


# You must be root to run this script.
is_root


# deal with a few verbos issues
[ ${VERBOSE} -gt 0 ] && VERBOPTS="-v"

# a few echos to make sure stuff is working.
[ ${VERBOSE} -gt "0" ] && {
	echo "EXECPATH = ${EXECPATH}"
	echo "PWD_PATH = ${PWD_PATH}"
	echo "ARCH = ${ARCH}"
	echo "BASEDIR = ${BASEDIR}"
	echo "DESTDIR = ${DESTDIR}"
	echo "WORKDIR = ${WORKDIR}"
	echo "BOOTDEST = ${BOOTDEST}"
	echo "ROOTDEST = ${ROOTDEST}"
	echo "KERNCONF = ${KERNCONF}"
	echo "DIRSKEL = ${DIRSKEL}"
	echo "FILELIST = ${FILELIST}"
}


#
# The main entry point of the script.
#


# make sure we are in correct location.
# When this script is called from jexec(1), the $PWD is always / (root) of the jail.
# This ensures that all the paths work correctly when executed in a jail, from outside the jail.

cd ${EXECPATH}



# make sure that the script will work.
# it is possible that none of the files are in place to work correctly.
# The script will still run without the files, and fail.




# make clean
info "Clean out old work."
[ -d ${WORKDIR} ] &&  chflags -R noschg ${WORKDIR} ; rm ${VERBOPTS} -rf ${WORKDIR}
[ -d ${ROOTDEST} ] &&  chflags -R noschg ${ROOTDEST}* ; rm ${VERBOPTS} -rf ${ROOTDEST}*
[ -d ${BOOTDEST} ] &&  chflags -R noschg ${BOOTDEST}* ; rm ${VERBOPTS} -rf ${BOOTDEST}*


# Do the dirs
info "Creating the Directory structure."
makedirs ${DIRSKEL} ${WORKDIR} || {
	case $? in
		255)	error 1 "MAKEDIRS: incorrect argument count" ;;
		254)	error 2 "MAKEDIRS: directory-list file does not exist, or not a file" ;;
	esac
}


# copy the files
info "Copying binary files to the work area"
linkcopy ${FILELIST} ${BASEDIR} ${WORKDIR} || {
	case $? in
		255)	error 3 "LINKCOPY: incorrect argument count." ;;
		254)	error 4 "LINKCOPY: file-list file does not exist, or not a file." ;;
		252)	error 5 "LINKCOPY: src dir does not exist, or not a dir." ;;
		248)	error 6 "LINKCOPY: destination dir does not exist, or not a dir." ;;
		*)		error 7 "LINKCOPY: unknown error." ;;
	esac
}


# copy the libs
info "Copying library files to the work area"
libcopy ${WORKDIR} ${LDDIRS} || {
	case $? in
		255)	error 8 "LIBCOPY: minimum number of args not passed." ;;
		254)	error 9 "LIBCOPY: arg 1, DESTDIR is not a directory." ;;
		*)		error 10 "LIBCOPY: unknown error." ;;
	esac
}


# copy and link the pam modules.
info "Copying PAM modules to the work area"
do_pam_modules




# copy the site files, specific to this build, or location.
[ -d ${DESTDIR}/site/ ] && {
	info "Copying site specific config files to the work area"
	(cd ${DESTDIR}/site/ ; cp ${VERBOPTS} -Rpf * ${WORKDIR})
}


# do the SSHD stuff.
do_sshd_stuff || {
	case $? in
		255)	error 11 "DO_SSHD_STUFF: error building ssh1 rsa key." ;;
		254)	error 11 "DO_SSHD_STUFF: error building ssh2 rsa key." ;;
		253)	error 11 "DO_SSHD_STUFF: error building ssh2 dsa key." ;;
		252)	error 11 "DO_SSHD_STUFF: error with sshd_config." ;;
		*)		error 11 "DO_SSHD_STUFF: unknown error." ;;
	esac
}


# do the login/passwd stuff.
info "Updating the user databse"
pwd_mkdb -p -d ${WORKDIR}/etc/ ${WORKDIR}/etc/master.passwd




# Code to change the root password.
do_root_pass ${WORKDIR}/etc/


# this would be an ideal place to insert any new users to the system.



# Do the kernel
info "Building the kernel."
build_kernel ${KERNCONF} ${BASEDIR} ${WORKDIR} || {
	case $? in
		255)	error 12 "BUILD_KERNEL: incorrect argument count." ;;
		254)	error 13 "BUILD_KERNEL: the kernel config doesn't exist, or not a file." ;;
		253)	error 14 "BUILD_KERNEL: base dir does not exist, or not a dir." ;;
		252)	error 15 "BUILD_KERNEL: destination dir does not exist, or not a dir." ;;
		251)	error 16 "BUILD_KERNEL: error copying the kernle config." ;;
		250)	error 17 "BUILD_KERNEL: error renaming the kernle config file." ;;
		249)	error 18 "BUILD_KERNEL: error with the config program." ;;
		248)	error 19 "BUILD_KERNEL: error in the make depend." ;;
		247)	error 20 "BUILD_KERNEL: error in the main kernel make." ;;
		246)	error 21 "BUILD_KERNEL: error installing the kernel." ;;
		*)		error 22 "BUILD_KERNEL: unknown error." ;;
	esac
}

# Here is a good place to do some config file stuff in /etc





# add some packages, experimental.



# check for extra packages to install.
[ -f ${DESTDIR}/wifibsd.pkgs ] && {
	for PKG in `cat ${DESTDIR}/wifibsd.pkgs`; do
		#info "Installing ${PKG} package."

		## this depends on the pkg_add command existing in the work area.
		#echo "chroot ${WORKDIR} /usr/sbin/pkg_add ${VERBOPTS} -R -r ${PKG}"
		#chroot ${WORKDIR} /usr/sbin/pkg_add ${VERBOPTS} -R -r ${PKG}
		#chroot ${WORKDIR} /bin/tcsh
		#read -p "press enter to continue" foo

	done
}


# Separate the boot dir for mfs-root systems.
info "Moving the /boot dir to: ${BOOTDEST}."
mv ${VERBOPTS} ${WORKDIR}/boot/ ${BOOTDEST}/



## The last chance to add any MFS specific things to the image.
#info "Altering the loader.rc to load the mfsroot."
#cat <<-EOF >> ${BOOTDEST}/loader.rc
#
#	echo "Loading MFS root."
#	load -t mfs_root /mfsroot
#
#EOF


# move the WORKDIR to ROOTDEST
info "Move the WORKDIR to the ROOTDEST."
mv ${VERBOPTS} ${WORKDIR}/ ${ROOTDEST}/


# create an archive of the root area.
info "Create a tar/gzip archive of ROOTDEST."
( cd ${ROOTDEST} ; tar ${VERBOPTS} -c -z -f ${DESTDIR}/root.tgz *)


## Create the mfsroot.gz image for the ROOTDEST.
#info "Creating the mfsroot image."
#create_mfsroot "${ROOTDEST}" "${DESTDIR}/mfsroot" "/mnt" || {
#	echo "create_mfsroot: code $?"
#	exit 9
#}
#
##ls -lh ${DESTDIR}/mfsroot
#
#info "Compressing the mfsroot image."
#[ -f ${DESTDIR}/mfsroot ] && gzip -9 ${DESTDIR}/mfsroot
#
#
## move the mfsroot.gz file to the BOOTDEST.
#info "Moving the mfsroot image to the BOOTDEST."
#mv ${VERBOPTS} ${DESTDIR}/mfsroot.gz ${BOOTDEST}


info "Compressing the boot image."
( cd ${DESTDIR} ; tar ${VERBOPTS} -c -z -f ${DESTDIR}/boot.tgz boot )








info "Removing trash"
chflags -R noschg ${BOOTDEST} ${ROOTDEST}
rm ${VERBOPTS} -rf ${BOOTDEST} ${ROOTDEST}

info "DONE."
