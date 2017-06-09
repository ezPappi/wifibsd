#!/bin/sh
# mkimage.sh
# a script to automate the creation of the wifibsd image.
# You cannot run this inside a jail, because mdconfig.


EXECPATH=${EXECPATH:-`dirname $(realpath $0)`}		# the is the location of this script.
PWD_PATH=`pwd`
BASEDIR=${BASEDIR:-"/"}						# The place all files are copied from.
DESTDIR=${DESTDIR:-"${PWD_PATH}"}			# Acontainer for our working directories.
WORKDIR=${DESTDIR}/work						# This is the main work area.
BOOTDEST=${DESTDIR}/boot					# The /boot and /rescue dirs live here (kern.flp).
ROOTDEST=${DESTDIR}/root					# A standard root filesystem (mfsroot.flp).
VERBOSE=${VERBOSE:-"0"}						# This controls the default output detail (verbosity).
DEVTYP=${DEVTYP:-"md"}						# This is for vnode memory disks on a 5.x system.
DEVNUM=${DEVNUM:-"8"}						# The default vnode memory-disk minor device number.
VNODE=${DEVTYP}${DEVNUM}					# The basename of the device-node used for the VNODE.
MNTPOINT=${MNTPOINT:-"/mnt"}				# The main mount point.
IMGROOT=${IMGROOT:-"${DESTDIR}/imgroot"}	# The dir we use for the final boot image.
BOOT_IMGNAME=${BOOT_IMGNAME:-"wifibsd-`date +%Y%m%d-%H%M`.bin"}	# The file becomes the final image.


# You can override any variable with your own configuration file.
[ -f ${DESTDIR}/wifibsd.conf ] && . ${DESTDIR}/wifibsd.conf


# include the sub-routines.
. ${EXECPATH}/wifibsd.subr || exit 1


# deal with a few verbos issues
[ ${VERBOSE} -gt 0 ] && VERBOPTS="-v"


# You must be root to run this script.
info "Checking for uid 0."
is_root


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


# make sure the imgroot exists
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
(cd ${ROOTDEST} ; tar -x -z -p -f ${DESTDIR}/root.tgz)		# Extract the data to the dir.
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

# we must have a folder which contains everything for the final image.
# we can calculate the final image size dynamicly.
BOOTIMGSIZE=`(cd ${IMGROOT} ; du -s . ) | cut -d "." -f 1 | cut -d " " -f 1`
BOOTIMGSIZE=`expr ${BOOTIMGSIZE} / 5 + ${BOOTIMGSIZE}`		# plus 1/5th



# Building the bootable image
# make the empty file, vnode-back it, label it, format, and mount it.
info "Creating a new vnode backed memory-disk for the bootable image file."
dd if=/dev/zero of=${BOOT_IMGNAME} count=${BOOTIMGSIZE} bs=1k 2> /dev/null
mdconfig -a -t vnode -u ${DEVNUM} -f ${BOOT_IMGNAME}


info "applying a bsd label to the /boot filesystem, and formating."
bsdlabel -Brw ${VNODE} auto
bsdlabel ${VNODE} | \
sed -e '/  c:/{ p;s/c:\(.*\)unused/a:\14.2BSD/; };' | \
bsdlabel -R ${VNODE} /dev/stdin
newfs -m 0 -b 8192 -f 1024 /dev/${VNODE}a


info "Mounting the BOOT filesystem and installing data."
mount ${VERBOPTS} /dev/${VNODE}a ${MNTPOINT}
(cd ${IMGROOT} ; tar -c -f - * ) | (cd ${MNTPOINT} ; tar ${VERBOPTS} -x -p -f - )


umount ${VERBOPTS} ${MNTPOINT}
mdconfig -d -u ${DEVNUM}



## if the DEST_DISK is defined, burn the image to it.
#[ -z ${DEST_DISK} ] || {
#	info "Burning the bootable image file dirrectly to the HDD/CF."
#	dd if=${BOOT_IMGNAME} of=/dev/`basename ${DEST_DISK}` bs=8k
#}

info "DONE."
