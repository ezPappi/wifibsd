\ Simple greeting screen, presenting basic options.
\ XXX This is far too trivial - I don't have time now to think
\ XXX about something more fancy... :-/
\ $FreeBSD: src/share/examples/bootforth/menuconf.4th,v 1.5 2000/09/05 16:30:09 dcs Exp $

: title
        f_single
        60 11 10 4 box
        29 4 at-xy 7 fg 15 bg
        ." Welcome to wifiBSD!"
        me
;

: menu
        2 fg
        20 7 at-xy
        ." 1.  Boot wifiBSD LiveCD."
    20 8 at-xy
    ." 2.  Install wifiBSD to your disk."
        20 9 at-xy
        ." 3.  Reboot."
        me
;

: tkey  ( d -- flag | char )
        seconds +
        begin 1 while
            dup seconds u< if
                drop
                -1
                exit
            then
            key? if
                drop
                key
                exit
            then
        repeat
;

: prompt
        14 fg
        20 12 at-xy
        ." Enter your option (1,2,3): "
        10 tkey
        dup 32 = if
            drop key
        then
        dup 0< if
            drop 49
        then
        dup emit  
        me
;
            
: help_text
        10 18 at-xy ." * Choose 1 to boot LiveCD."
        10 19 at-xy ." * Choose 2 to install wifibsd to your HD/CF/USB disk."
        10 20 at-xy ." * Choose 3 in order to warm boot your machine."
;

: (reboot) 0 reboot ;
        
: main_menu
        begin 1 while
                clear
                f_double
                79 23 1 1 box
                title
                menu
                help_text
                prompt
                cr cr cr
                dup 49 = if
                        drop
                        1 25 at-xy cr
                        ." Proceeding with standard boot. Please wait..." cr
                        0 boot-conf exit
                then
                dup 50 = if
                        drop
                        1 25 at-xy cr
                        ." Proceeding to install wifibsd to your disk. Please wait..." cr  
                        s" /boot/install.conf" read-conf
                        0 boot-conf exit
                then
                dup 51 = if
                        drop
                        1 25 at-xy cr
                        ['] (reboot) catch abort" Error rebooting"
                then
                20 12 at-xy
                ." Key " emit ."  is not a valid option!"
                20 13 at-xy
                ." Press any key to continue..."
                key drop
        repeat
;

