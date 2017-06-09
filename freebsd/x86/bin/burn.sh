#!/bin/sh
# This script is written by yazzy@yazzy.org meant to be used on FreeBSD and Linux.
# It's goal is to create a convinient way to burn misc file types with command line.
# It uses libdialog for the funky menu.
# It will also install cdrecord and cdrdao from the ports collection or as a binary package if not present.
# Thanks masta for his ideas and assisting me with this one and other scripts.

export PATH="${HOME}/bin:/bin:/sbin:/usr/bin/usr/sbin:${PATH}"
 
DIALOG=${DIALOG:-`which dialog`}			# Where we have dialog executable
CDRECORD=${CDRECORD:-`which cdrecord`}			# Where to find cdrecord
CDRDAO=${CDRDAO:-`which cdrdao`}			# Location of cdrdao
GROWISOFS=${GROWISOFS:-`which growisofs`}
CHECKLIST=`mktemp /tmp/checklist.tmp.XXXXXXXX`		# Temp file
PATH="/fridge/burn"					# Where I keep my files I burn
DEV="/dev/cdroms/cdrom0"				# SCSI device
SPEED_RW="8"						# Desired CDRW speed
SPEED_BLANK="2"						# How fast to blank your CD

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
	Press SPACE to chose an option. 
  
	Which of the following do you want to do?
EOF
`
  
$DIALOG --title "CD/DVD Burning." --clear \
	--radiolist "${MAINMENU}" -1 -1 5 \
	"ISO"   "Burn .iso image."  off \
	"BIN"   "Burn .bin file."   off \
	"DVD"   "Burn directory to DVD."   off \
	"WAVs"  "Burn *.wav files." off \
	"Blank" "Blank your CDRW."  off 2> ${CHECKLIST} || exit $?

CHOICE=`cat ${CHECKLIST}`
rm -f ${CHECKLIST}

echo "CHOICE = ${CHOICE}"
  

case ${CHOICE} in

ISO)
	echo "'${CHOICE}' chosen. Burning ISO."
	for x in *.iso; 
	do ${CDRECORD} -v dev=${DEV} fs=48m speed=${SPEED_RW} -v -eject -pad -data "$x"  >|/tmp/ERRORS$$ 2>&1
	done  
# zero status indicates burning was successful
        if [ "$?" = "0" ]
            then
            dialog --title "Burning of ISO file" --msgbox "Burning of your ISO file completed successfully." 10 50
        else
        # Burning of ISO failed, display error log
            dialog --title "Burning" --msgbox "Burning failed  -- Press <Enter> to see error log." 10 50
                dialog --title "Error Log" --textbox /tmp/ERRORS$$ 22 83

fi
        rm -f /tmp/ERRORS$$
        clear

;;

BIN)
	echo " '${CHOICE}' chosen. Burning BIN."
	for x in *.cue; do
	${CDRDAO} write --device ${DEV} --eject -v 1 "$x"  >|/tmp/ERRORS$$ 2>&1
	done 
# zero status indicates burning was successful
        if [ "$?" = "0" ]
            then
            dialog --title "Burning of BIN file" --msgbox "Burning of your BIN file completed successfully." 10 50
        else
        # Burning of BIN failed, display error log
            dialog --title "Burning" --msgbox "Burning failed  -- Press <Enter> to see error log." 10 50
                dialog --title "Error Log" --textbox /tmp/ERRORS$$ 22 82

fi
        rm -f /tmp/ERRORS$$
        clear

;;


DVD)
	echo " '${CHOICE}' chosen. Burning DVD."
	${GROWISOFS} -Z ${DEV} -R -J ${PATH}  >|/tmp/ERRORS$$ 2>&1
# zero status indicates burning was successful
        if [ "$?" = "0" ]
            then
            dialog --title "Burning of DVD" --msgbox "Burning of your DVD completed successfully." 10 50
        else
        # Burning of BIN failed, display error log
            dialog --title "Burning" --msgbox "Burning failed  -- Press <Enter> to see error log." 10 50
                dialog --title "Error Log" --textbox /tmp/ERRORS$$ 22 82

fi
        rm -f /tmp/ERRORS$$
        clear

;;


WAVs)
	echo "'${CHOICE}' chosen. Burning music CD."
	${CDRECORD} -v speed=${SPEED_RW} dev=${DEV} -eject -audio -pad *.wav  >|/tmp/ERRORS$$ 2>&1

# zero status indicates burning was successful
        if [ "$?" = "0" ]
            then
            dialog --title "Burning of music CD" --msgbox "Burning of your music CD completed successfully." 10 50
        else
        # Burning of music CD failed, display error log
            dialog --title "Burning" --msgbox "Burning failed  -- Press <Enter> to see error log." 10 50
                dialog --title "Error Log" --textbox /tmp/ERRORS$$ 22 83

fi
        rm -f /tmp/ERRORS$$
        clear

;;

Blank)
#	dialog --title "INFO" --infobox "${CHOICE} chosen. Please wait. Blanking CDRW" 10 30 ; sleep 4
	echo "'${CHOICE}' chosen. Blanking CDRW"

#Note: blank=fast may not work on every CDRW. In cases like that use blank=all instead
	${CDRECORD} -v speed=${SPEED_BLANK} dev=${DEV} blank=fast >|/tmp/ERRORS$$ 2>&1 && 
#dialog --title "Blanking status" --textbox tail -F /tmp/ERRORS$$ 10 30 ;
# zero status indicates burning was successful
	if [ "$?" = "0" ]
	    then
	    dialog --title "Blanking" --msgbox "Blanking of your CDRW completed successfully." 10 50
	else
	# Blanking failed, display error log
	    dialog --title "Blanking" --msgbox "Blanking failed  -- Press <Enter> to see error log." 10 50
		dialog --title "Error Log" --textbox /tmp/ERRORS$$ 22 72

fi
	rm -f /tmp/ERRORS$$
	clear
;;
  
esac

#
# Exit with no errors.
#
        
exit 0



