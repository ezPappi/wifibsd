#!/bin/sh
# mkcdrom.sh
# a script to automate the creation of the wifibsd image.
# You cannot run this inside a jail, because mdconfig.


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
IMGROOT=${IMGROOT:-"${DESTDIR}/cdroot"}			# The dir mkisofs uses for the cd9660 image.
CDIMAGE=${CDIMAGE:-"${DESTDIR}/wifibsd-`date +%Y%m%d-%H%M`.iso"}	# The name of the cd9660 file for mkisofs to produce.
CDVERSION="WifiBSD"



# You can override any variable with your own configuration file.
[ -f ${DESTDIR}/wifibsd.conf ] && . ${DESTDIR}/wifibsd.conf


# include the sub-routines.
. ${EXECPATH}/wifibsd.subr || exit 1


# deal with a few verbos issues
[ ${VERBOSE} -gt 0 ] && VERBOPTS="-v"


# You must be root to run this script.
info "Checking for uid 0."
is_root


# Thanks to JKH for the idea.
type mkisofs 2>&1 | grep " is " >/dev/null || {
	info "The mkisofs program is not installed.  Attempting to fetch it."
	if	[ -f /usr/ports/sysutils/cdrtools/Makefile ]; then
			( cd /usr/ports/sysutils/mkisofs && make install && make clean )
	else	pkg_add -r cdrtools || {
				echo "Could not get it via pkg_add - please go install this"
				echo "from the ports collection and run this script again."
				exit 255
			}
	fi
}


# make clean
info "Clean out old work."
[ -d ${IMGROOT} ] && rm -rf ${IMGROOT}
[ -d ${ROOTDEST} ] && {
	chflags -R noschg ${ROOTDEST}
	rm -rf ${ROOTDEST}
}
	
# make sure our mount point exists.
info "Checking if the working mountpoint exist."
[ -d ${MNTPOINT} ] || mkdir -p ${MNTPOINT}


# make sure the IMGROOT exists
[ -d ${IMGROOT} ] || mkdir -p ${IMGROOT}


ROOT_IMGNAME=${DESTDIR}/mfsroot		# Always name it "mfsroot"

## sanity check
[ ${VERBOSE} -gt "0" ] && {
	echo "EXECPATH = ${EXECPATH}"
	echo "DESTDIR = ${DESTDIR}"
	echo "BOOTDEST = ${BOOTDEST}"
	echo "ROOTDEST = ${ROOTDEST}"
	echo "DEVTYP = ${DEVTYP}"
	echo "DEVNUM = ${DEVNUM}"
	echo "VNODE = ${VNODE}"
	echo "MNTPOINT = ${MNTPOINT}"
	echo "MFSIMGSIZE = ${MFSIMGSIZE}"
	echo "ROOT_IMGNAME = ${ROOT_IMGNAME}"
}


# Go ahead and build the MFS.
info "Building the MFS root image file."
mkdir -p ${ROOTDEST}										# Create the working dir.
info
(cd ${ROOTDEST} ; tar -x -z -p -f ${DESTDIR}/root.tgz)		# Extract the data to the dir.

info "foo 1"
create_mfs ${ROOTDEST} ${ROOT_IMGNAME} ${MNTPOINT} ${MFSIMGSIZE}	# Produce the mfsroot.

info "Compressing thee MFS root image file."
gzip ${VERBOPTS} -9 ${ROOT_IMGNAME}							# This might be a long while.


# This is the main work, it copies all the files to the IMGROOT area.
info "Copying data to the imgroot area."
(cd ${IMGROOT} ; tar ${VERBOPTS} -x -z -p -f ${DESTDIR}/boot.tgz)		# MFS is enabled.
mv ${VERBOPTS} ${ROOT_IMGNAME}.gz ${IMGROOT}			# Move the mfs image to the boot image.





###
### boot stuff
###

# make sure the boot/cdboot program exists.
[ -f ${IMGROOT}/boot/cdboot ] || cp -v -Rpf /boot/cdboot /${IMGROOT}/boot/




###
### Make the cd9660 filesystem.
###

# The cdboot style.
CDBOOTOPT="-b boot/cdboot -no-emul-boot"



info "Creating the iso cd9660 filesystem image."
#rock-ridge, and Joliet.
mkisofs -o ${CDIMAGE} -R -J \
-V "${CDVERSION}" \
${CDBOOTOPT} \
${IMGROOT}

info "DONE."

