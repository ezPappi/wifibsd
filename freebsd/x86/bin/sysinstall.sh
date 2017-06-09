#!/bin/sh

# a script to install wifibsd with dialog.


DIALOG=${DIALOG:-"/usr/bin/dialog"}
CHECKLIST=`mktemp /tmp/$(basename $0).XXXXX`

MAINMENU=`cat <<-EOF
	This is simple frontend to install WifiBSD.
	Press SPACE to toggle an option on/off.

	which of the following do you want to do?
EOF
`
while : ; do

	$DIALOG --title "WifiBSD System Installer." --clear \
		--menu "${MAINMENU}" -1 -1 13 \
		"1"		"Create Dirs"   \
		"2"		"Copy the Files." \
		"3"		"Copy the libs." \
		"4"		"Copy and link the pam modules." \
		"5"		"Copy the site files." \
		"6"		"Do the SSHD stuff." \
		"7"		"Do the passwd database stuff." \
		"8"		"Change the root password." \
		"9"		"Build the kernel." \
		"10"	"Compress the kernel." \
		"11"	"Install extra packages." \
		"12"	"Finalize." 2> ${CHECKLIST} || exit $?

	CHOICE=`cat ${CHECKLIST}`
	rm -f ${CHECKLIST}

	#case ${CHOICE} in
	#	ISO)	echo "'${CHOICE}' chosen. Burning ISO."
	#			for x in *.iso;
	#				do ${CDRECORD} -v dev=0,4,0 fs=48m speed=12 -v -eject -pad -data "$x"
	#			done
	#	;;
	#	BIN)	echo "'${CHOICE}' chosen. Burning BIN."
	#			for x in *.cue; do
	#				cdrdao write --device 0,4,0 --eject -v 1 "$x"
	#			done
	#	;;
	#	WAVs)	echo "'${CHOICE}' chosen. Burning music CD."
	#			${CDRECORD} -v speed=12 dev=0,4,0 -eject -audio -pad *.wav
	#	;;
	#	Blank)	echo "'${CHOICE}' chosen. Blankind CDRW"
	#			${CDRECORD} -v speed=4 dev=0,4,0 blank=fast
	#	;;
	#esac

done