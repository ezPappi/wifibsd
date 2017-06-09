#!/bin/sh

DIALOG=${DIALOG:-"/usr/bin/dialog"}
CDRECORD=${CDRECORD:-"/usr/local/bin/cdrecord"}
CHECKLIST=`mktemp /tmp/$(basename $0).XXXXX`

# Thanks to JKH for the idea.
type cdrecord 2>&1 | grep " is " >/dev/null || {
	if	[ -f /usr/ports/sysutils/cdrtools/Makefile ]; then
			( cd /usr/ports/sysutils/cdrtools && make install && make clean )
	else	pkg_add -r cdrtools || {
				echo "Could not get it via pkg_add - please go install this"
				echo "from the ports collection and run this script again."
				exit 255
			}
	fi
}

MAINMENU=`cat <<-EOF
	This is simple frontend to burn CDs.
	You can use it to burn ISO and BIN images 
	in your current directory. 
	Press SPACE to toggle an option on/off. 
	
	which of the following do you want to do?
EOF
`

$DIALOG --title "SCSI CD Burning." --clear \
	--radiolist "${MAINMENU}" -1 -1 5 \
	"ISO"   "Burn .iso image."  on \
	"BIN"   "Burn .bin file."   off \
	"WAVs"  "Burn *.wav files." off \
	"Blank" "Blank your CDRW."  off 2> ${CHECKLIST} || exit $?

CHOICE=`cat ${CHECKLIST}`
rm -f ${CHECKLIST}

case ${CHOICE} in
	ISO)	echo "'${CHOICE}' chosen. Burning ISO."
			for x in *.iso; 
				do ${CDRECORD} -v dev=0,4,0 fs=48m speed=12 -v -eject -pad -data "$x"
			done 
	;;
	BIN)	echo "'${CHOICE}' chosen. Burning BIN."
			for x in *.cue; do
				cdrdao write --device 0,4,0 --eject -v 1 "$x"
			done 
	;;
	WAVs)	echo "'${CHOICE}' chosen. Burning music CD."
			${CDRECORD} -v speed=12 dev=0,4,0 -eject -audio -pad *.wav 
	;;	
	Blank)	echo "'${CHOICE}' chosen. Blankind CDRW"
			${CDRECORD} -v speed=4 dev=0,4,0 blank=fast 
	;;
esac
