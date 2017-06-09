#!/usr/local/bin/bash

# Create directories and copy files needed for the CF image


BUILD_SKEL=/			# Where do we copy files from
TARGET_DIR=install_ng
FILELIST=cf_files


copyfiles()
{
    typeset fl=""

    SRCDIR=$1
    TARGETDIR=$2
    FILELIST=$3

    cat "$FILELIST" | while read typ perms own f
    do
        fl="$f"

        if [[ "$typ" = "d" ]]
        then
                if [[ ! -d $f ]]
                then
                        mkdir -m $perms -p "$TARGETDIR/$f"
                else
                        continue
                fi

        elif [[ "$typ" = "f" ]]
        then
                /bin/cp -f -p "$SRCDIR/$f" "$TARGETDIR/$f"

        elif [[ "$typ" = "s" ]]
        then
                fl=`echo $f | sed 's/=/ /' | cut -d" " -f1`
                trg=`echo $f | sed 's/=/ /' | cut -d" " -f2`
                ln -sf $trg "$TARGETDIR/$fl"

        elif [[ "$typ" = "l" ]]
        then
                fl=`echo $f | sed 's/=/ /' | cut -d" " -f1`
                trg=`echo $f | sed 's/=/ /' | cut -d" " -f2`

                dir=`pwd`
                cd `dirname "$TARGETDIR/$fl"`
                ln -f $trg `basename "$fl"`
                cd $dir
        else
                continue
        fi

        if [[ "$typ" != "s" ]]; then
		if [[ "$typ" != "d" ]]; then
			/bin/chmod $perms "$TARGETDIR/$fl"
            	fi
		/usr/sbin/chown $own "$TARGETDIR/$fl"
        else
			/usr/sbin/chown -h $own "$TARGETDIR/$fl"
        fi
    done
}


checklibs()
{
    typeset fl=""

    SRCDIR=$1
    TARGETDIR=$2
    FILELIST=$3

    cat "$FILELIST" | while read typ perms own f
    do
        fl="$f"

        if [[ "$typ" = "d" ]] ; then
		    cd $f ; ldd -f "%p\n" *|sort|uniq >> /tmp/liblist
	    fi
    done
}

copyfiles "$BUILD_SKEL" "$TARGET_DIR" $FILELIST
