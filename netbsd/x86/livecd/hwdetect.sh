#!/bin/sh

get_disks () {
# Return list of available devices
DISK_NAMES=`mktemp /tmp/checklist.tmp.XXXXXXXX`
TEMP=`mktemp /tmp/checklist.tmp.XXXXXXXX`

DRIVES=`sysctl hw.disknames`

for x in ${DRIVES} ; do
        # Display only output equal 3 chars, e.g wd0 or ad0
        if [ ${#x} = 3 ]; then
                case ${x} in
                fd*) ;;
                md*) ;;
#                cd*) ;;
                *) echo ${x} |sort -n >> ${DISK_NAMES} || exit $?
                ;;
                esac
        fi
done

}

print_disks () {
	get_disks
	echo "Detected drives:"
for a in `cat ${DISK_NAMES}` ; do
	echo "Disk ${a} - Size: XYZ MB" 
done
}

chose_drive () {
print_disks 

echo ""
read -p "Which drive do you want to install wifiBSD on?: " drive_choice
echo ""

for b in `cat ${DISK_NAMES}` ; do
	echo $b >> ${TEMP}
done

good=$(cat ${TEMP})
echo $good
#input="cd0"

case " $good " in 
*\ $drive_choice\ *) echo yes
;; 
*) echo no
;; 
esac


#var=$(grep strings ${DISK_NAMES})
#(
#while read $drive_choice; do 
#	if [ "${drive_choice}" = "${a}" ]; then 
#		echo "You chose to use ${drive_choice}, proceeding..."; 
#	else
#		echo "** Drive ${drive_choice} does not exists **"
#		echo "** Please try again **"
#	fi; 
#done ) < ${DISK_NAMES}

# ${DISK_NAMES} has a list of device names in separate lines, eg. wd0 cd0
}

chose_drive

#while read L; do <do something to L to make it suitable for compare>; [[ $OTHERVAR == $L ]] && perform action; done < file
#VARLIST="$VAR1 $VAR2 $VAR3"; for I in $VARLIST; do [[ $OTHERVAR == $I ]] && perform action; done
#( while read Y; do if [ "$X" = "$Y" ]; then lalala; fi; done ) < file


#echo $drive_choice | grep -q $a 
#if [ $? -eq 0 ] ; then 
#echo "succes"  
#else echo "no" 
#fi

