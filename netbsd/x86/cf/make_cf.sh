#!/bin/sh

VERSION="0.12"
MYKERNEL="WIFIBSD_LIVE_CF"		# Name of the kernel
IMG_DIR="install"			# Dir om the build
CF_IMG_SIZE="16384"			# Size of the CF image
FS_SIZE="16384k"			# Size of the file system
MY_BUILD_DIR=`pwd`			# Path to my build
MY_IMAGE="wifibsd-nbsd-cf-${VERSION}.bin"	# Name of the CF image
KERNEL_NAME="wifibsd"
PLATFORM="i386"

PWD_PATH=`pwd`

. ${PWD_PATH}/colors.disp

# Create RAM disk:

showmsg()
{
        echo "${YELLOW}===> $@${NORMAL}"
}

showerr()
{
        echo "${RED}===> $@${NORMAL}"
}
showinfo()
{
        echo "${BLUE}===> $@${NORMAL}"
}

showprogress()
{
        echo "${GREEN}===> $@${NORMAL}"
}

handle_site() {
		showprogress "Removing old site.tar.gz from ${IMG_DIR}"
                rm -f ${IMG_DIR}/site.tar.gz
		showprogress "Compressing site into site.tar.gz"
                tar -zcpf site.tar.gz site
		showprogress "Moving site.tar.gz to ${IMG_DIR}"
                mv site.tar.gz ${IMG_DIR}
}

build() {

        if [ -x /usr/obj/sys/arch/${PLATFORM}/compile/${MYKERNEL}/netbsd ]; then
                cp /usr/obj/sys/arch/${PLATFORM}/compile/${MYKERNEL}/netbsd ${MY_BUILD_DIR}
		showmsg "Renaming kernel from netbsd to ${KERNEL_NAME}"
		mv netbsd ${KERNEL_NAME}
	handle_site
		makefs -s ${FS_SIZE} -t ffs md.img ${IMG_DIR}
		mdsetimage -v -s ${KERNEL_NAME} md.img
		gzip -f -9 ${KERNEL_NAME} ; mv ${KERNEL_NAME}.gz ${KERNEL_NAME}

		dd if=/dev/zero of=${MY_IMAGE} bs=1k count=${CF_IMG_SIZE}
		vnconfig -v -c vnd0 ${MY_IMAGE}
		disklabel -R -r vnd0 disklabel.proto	# Read disklabel from disklabel.proto
#		disklabel -e -I vnd0a			# Edit disklabel by hand
		newfs -m 0 vnd0a
#		newfs -m 0 vnd0b
	/usr/sbin/installboot -v -m ${PLATFORM} -o timeout=1,console=com0 -t ffs /dev/rvnd0a ${MY_BUILD_DIR}/bin/bootxx_ffsv1_install
#	/usr/sbin/installboot -v -m ${PLATFORM} -o timeout=3,console=pc -t ffs /dev/rvnd0a ${MY_BUILD_DIR}/bin/bootxx_ffsv1_install
		mount /dev/vnd0a /mnt
		cp ${MY_BUILD_DIR}/bin/boot /mnt/ ; cp ${KERNEL_NAME} /mnt/
		umount /mnt
		vnconfig -u vnd0
		showmsg "SCPing image file to remote host"
#               scp ${MY_IMAGE} wifibsd@fr2.eu.wifibsd.org:
#		scp ${MY_IMAGE} yazzy@192.168.2.204:
#		scp ${MY_IMAGE} yazzy@lapdance:
#		scp ${MY_IMAGE} yazzy@urukhai:
        else
                showerr "Compile your kernel first!"
fi
#        clear

}


firmware() {
	cp /usr/src/sys/arch/i386/compile/obj/${MYKERNEL}/netbsd  .
	mv netbsd wifibsd
	makefs -s 16384k -t ffs md.img site/
	mdsetimage -v -s wifibsd md.img
	gzip -f -9 wifibsd
	mv wifibsd.gz wifibsd.pkg

}

cleanup() {
	showinfo "Cleaning up old files"
	if [ -f ${KERNEL_NAME} ]; then
		rm -f ${KERNEL_NAME}
	fi

	if [ -f md.img ]; then
		rm -f md.img
	fi

	if [ -e ${MY_IMAGE} ]; then
		rm -f ${MY_IMAGE}
	fi
}

cleanup

build

cleanup

#
# Exit with no errors.
#

exit 0
