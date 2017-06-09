#!/bin/sh

DSK=wd0
label=/home/yazzy/devel/netbsd/x86/branches/current/cf/label

eval `fdisk -S ${DSK} 2>/dev/null`
DLCYL=${DLCYL}
DLHEAD=${DLHEAD}
DLSEC=${DLSEC}
DLSIZE=${DLSIZE}
BCYL=${BCYL}
BHEAD=${BHEAD}
BSEC=${BSEC}
PART0ID=${PART0ID}
PART0SIZE=${PART0SIZE}
PART0START=${PART0START}
PART0FLAG=${PART0FLAG}
PART0BCYL=${PART0BCYL}
PART0BHEAD=${PART0BHEAD}
PART0BSEC=${PART0BSEC}
PART0ECYL=${PART0ECYL}
PART0EHEAD=${PART0EHEAD}
PART0ESEC=${PART0ESEC}
PART1SIZE=${PART1SIZE}
PART2SIZE=${PART2SIZE}
PART3SIZE=${PART3SIZE}

PART0START=${BSEC}
P0SIZE=$((${DLSIZE}-${PART0START}))
PART0ID=169

#
# Create disk label /etc/label
#
echo "# /dev/rwd0d:" > ${label} 2>/dev/null
echo "type: unknown" >> ${label} 2>/dev/null
echo "disk: wifibsd" >> ${label} 2>/dev/null
echo "label: fictitious" >> ${label} 2>/dev/null
echo "flags:" >> ${label} 2>/dev/null
echo "bytes/sector: 512" >> ${label} 2>/dev/null
echo "sectors/track: ${BSEC}" >> ${label} 2>/dev/null
echo "tracks/cylinder: ${DLHEAD}" >> ${label} 2>/dev/null
echo "sectors/cylinder: 1008" >> ${label} 2>/dev/null
echo "cylinders: ${DLCYL}" >> ${label} 2>/dev/null
echo "total sectors: ${DLSIZE}" >> ${label} 2>/dev/null
echo "rpm: 3600" >> ${label} 2>/dev/null
echo "interleave: 1" >> ${label} 2>/dev/null
echo "trackskew: 0" >> ${label} 2>/dev/null
echo "cylinderskew: 0" >> ${label} 2>/dev/null
echo "headswitch: 0           # microseconds" >> ${label} 2>/dev/null
echo "track-to-track seek: 0  # microseconds" >> ${label} 2>/dev/null
echo "drivedata: 0" >> ${label} 2>/dev/null
echo "" >> ${label} 2>/dev/null
echo "4 partitions:" >> ${label} 2>/dev/null
echo "#        size    offset     fstype [fsize bsize cpg/sgs]" >> ${label} 2>/dev/null
echo " a:  ${P0SIZE}        ${BSEC}     4.2BSD      0     0     0  # (Cyl.      0 -     25*)" >> ${label} 2>/dev/null
echo " d:  ${DLSIZE}         0     unused      0     0        # (Cyl.      0 -     25*)" >> ${label} 2>/dev/null
echo "" >> ${label} 2>/dev/null

