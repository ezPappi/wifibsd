#!/bin/sh


# a wifibsd script to install wifibsd over PXE
# thanks to Matt Simerson for his PXE netboot recipy.




EXECPATH=${EXECPATH:-`dirname $(realpath $0)`}		# the is the location of this script.
PWD_PATH=`pwd`
BASEDIR=${BASEDIR:-"/"}						# The place all files are copied from.
DESTDIR=${DESTDIR:-"${PWD_PATH}"}			# A container for our working directories.
WORKDIR=${DESTDIR}/work						# This is the main work area.
BOOTDEST=${DESTDIR}/boot					# The /boot and /rescue dirs live here (kern.flp).
ROOTDEST=${DESTDIR}/root					# A standard root filesystem (mfsroot.flp).
VERBOSE=${VERBOSE:-"0"}						# This controls the default output detail (verbosity).
DEVTYP=${DEVTYP:-"md"}						# This is for vnode memory disks on a 5.x system.
DEVNUM=${DEVNUM:-"6"}						# The default vnode memory-disk minor device number.
VNODE=${DEVTYP}${DEVNUM}					# The basename of the device-node used for the VNODE.
MNTPOINT=${MNTPOINT:-"/mnt"}				# The main mount point.
#BOOT_IMGNAME=${BOOT_IMGNAME:-"wifibsd-`date +%Y.%m.%d.%H.%M`.iso"}	# The file becomes the final image.


# You can override any variable with your own configuration file.
[ -f ${DESTDIR}/wifibsd.conf ] && . ${DESTDIR}/wifibsd.conf


# include the sub-routines.
. ${EXECPATH}/wifibsd.subr || exit 1



# deal with a few verbos issues
[ ${VERBOSE} -gt 0 ] && VERBOPTS="-v"


# You must be root to run this script.
is_root


# Create the TFTP root area.
# this place will contain /boot, the pxeboot program, and our mfsroot.

mkdir -p /tftpboot/boot

cp -Rpf /boot/pxeboot /tftpboot/



# copy the wifibsd boot.tgz into the /tftpboot/boot


#copy over the mfsroot.gz image to the /tftpboot.
