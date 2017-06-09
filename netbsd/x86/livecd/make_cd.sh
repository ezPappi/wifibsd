#!/bin/sh

VERSION="0.12"
publisher_id="wifiBSD"
MYKERNEL="WIFIBSD_LIVE_CD"			# Name of the kernel
IMG_DIR="install"				# Dir of the build
ISO_DIR="cdimage"				# Dir of the build
CF_IMG_SIZE="5760"				# Size of the CF image
FS_SIZE="16285696"				# Size of the file system
#FS_SIZE="3514368"				# Size of the file system
MY_BUILD_DIR=`pwd`				# Path to my build
#MY_IMAGE="wifibsd.fs"				# Name of the CF image
MY_IMAGE="wifibsd-nbsd-livecd-${VERSION}" 	# Name of the CF image
KERNEL_NAME="wifibsd"				# Name of our kernel
PLATFORM="i386"
VERBOSE=""
: ${MKISOFS:=/usr/local/bin/mkisofs}
: ${MKISOFS_FIXED_ARGS:=-no-emul-boot -boot-load-size 30 -boot-info-table}
: ${MKISOFS_ARGS:=-l -J -R -nobak -no-emul-boot ${VERBOSE} -V wifibsd-${VERSION} -sysid wifibsd -publisher wifibsd.org -p yazzy@wifibsd.org }

PWD_PATH=`pwd`

. ${PWD_PATH}/colors.disp

# ====================================================================== #
#  				My functions :                           #
# ====================================================================== #

showmsg()
{
        echo "${YELLOW}===> $@${NORMAL}"
}

showprogress()
{
        echo "${GREEN}===> $@${NORMAL}"
}

create_iso(){
#mkisofs -l -J -R -o wifibsd.iso -c boot.catalog -b cdboot -nobak -no-emul-boot cdimage
${MKISOFS} ${MKISOFS_ARGS} \
-o ${MY_IMAGE}.iso \
-c boot.catalog -b cdboot \
${ISO_DIR}
#-V ${VERSION} \
#-P ${publisher_id} \
}

# Create RAM disk:

handle_site() {
                showprogress "Removing old system.wpkg from ${IMG_DIR}"
                rm -f ${ISO_DIR}/system.wpkg
                showprogress "Compressing site into system.wpkg"
		cd ../cf
                tar -zcpf system.wpkg site 
                showprogress "Moving system.wpkg to ${ISO_DIR}"
                mv system.wpkg ../livecd/${ISO_DIR}/
		cd ../livecd/
}

build() {

	if [ -x /usr/obj/sys/arch/${PLATFORM}/compile/${MYKERNEL}/netbsd ]; then
        	cp /usr/obj/sys/arch/${PLATFORM}/compile/${MYKERNEL}/netbsd ${MY_BUILD_DIR}
		showmsg "Renaming kernel from netbsd to ${KERNEL_NAME}"
		mv netbsd ${KERNEL_NAME}
		showmsg "Creating image of file system."
		makefs -s ${FS_SIZE} -f 7166 -t ffs md.img ${IMG_DIR}
		showmsg "Inserting image into kernel."
		mdsetimage -v -s ${KERNEL_NAME} md.img 
		showmsg "Compressing kernel."
		gzip -f -9 ${KERNEL_NAME} ; mv ${KERNEL_NAME}.gz ${KERNEL_NAME}
#		showmsg "Creating virtual disk and file system on it."
#		dd if=/dev/zero of=${MY_IMAGE} count=${CF_IMG_SIZE}
#		vnconfig -t floppy288 -v -c /dev/vnd0d ${MY_IMAGE} || exit
#		disklabel -rw /dev/vnd0d floppy288 || showmsg "Disklabel problem" $
#		newfs -m 0 -o space -i 204800 /dev/rvnd0a

#		vnconfig -v -c vnd0d ${MY_IMAGE} || exit
#		disklabel -R -r vnd0d disklabel.proto	|| showmsg "Disklabel problem" $ # Read disklabel from disklabel.proto 
#		disklabel -e -I vnd0a			# Edit disklabel by hand
#		newfs -m 0 vnd0a 
#		newfs -m 0 vnd0b  
#	/usr/sbin/installboot -v -m ${PLATFORM} -o timeout=3,console=com0 -t ffs /dev/rvnd0a ${MY_BUILD_DIR}/bin/bootxx_ffsv1_install
#		showmsg "Bootstraping virtual disk"
#		/usr/sbin/installboot -v -m ${PLATFORM} \
#		-o timeout=3,console=pc \
#		-t ffs /dev/rvnd0a ${MY_BUILD_DIR}/bin/bootxx_ffsv1

#		showmsg "Mouting virtual disk on /mnt, copying our files and umouting it."
#		mount /dev/vnd0a /mnt
#		cp ${MY_BUILD_DIR}/bin/boot /mnt/ ; cp ${KERNEL_NAME} /mnt/
#		umount /mnt
#		vnconfig -u vnd0d
#		cp ${MY_IMAGE} ${ISO_DIR}/
		cp ${KERNEL_NAME} ${ISO_DIR}/

		showmsg "Copying site files..."		
		handle_site
                showmsg "Creating ISO CD9660 image"
		create_iso

		echo ""		
		showmsg "SCPing ${KERNEL_NAME}.iso to remote site"	
#		scp ${MY_IMAGE}.iso wifibsd@fr2.eu.wifibsd.org:
		scp ${MY_IMAGE}.iso yazzy@192.168.99.4:
#		scp ${MY_IMAGE}.iso yazzy@lapdance:
#		scp ${MY_IMAGE}.iso yazzy@urukhai:

		echo ""
		showmsg "DONE !"
        else    
                showmsg "Compile your kernel first!"
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
	showmsg "Cleaning up..." 
	if [ -f ${KERNEL_NAME} ]; then
		rm -f ${KERNEL_NAME}
	fi

	if [ -f md.img ]; then
		rm -f md.img
	fi

	if [ -e ${MY_IMAGE} ]; then
		rm -f ${MY_IMAGE}
	fi

        if [ -f ${MY_IMAGE}.iso ]; then
                rm -f ${MY_IMAGE}.iso
        fi
	
	if [ -f ${ISO_DIR}/${MY_IMAGE} ]; then
		rm -f cdimage/${MY_IMAGE} 
	fi
}

cleanup

build

cleanup

# 
# Exit with no errors.
#

exit 0


