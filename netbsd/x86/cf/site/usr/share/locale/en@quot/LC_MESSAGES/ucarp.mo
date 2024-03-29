��    0      �  C         (     )  �  D     -	     =	  m   L	     �	     �	     �	     
     
     .
  )   E
  D   o
     �
     �
     �
  '   �
           <     M     i     �  &   �  0   �  -   �  "   %  3   H  (   |  $   �  "   �      �          .     I  !   ^  *   �  (   �  #   �     �  +     '   ?  &   g  *   �     �  $   �     �       V  &     }  �  �     �     �  m   �          !     :     U     j     �  )   �  D   �               5  '   G      o     �     �     �     �  &   �  0     -   K  "   y  3   �  (   �  $   �  "         A     b     �     �  !   �  *   �  (   �  #   (     L  +   g  '   �  &   �  *   �       $   -     R     g        	   /      !       &       +      
                      %          .                       "             0              )   (                       '       ,                 -          $             #      *                          * advertisement injected * --interface=<if> (-i <if>) : bind interface <if>
--srcip=<ip> (-s <ip>) : source (real) IP address of that host
--vhid=<id> (-v <id>) : virtual IP identifier (1-255)
--pass=<pass> (-p <pass>) : password
--preempt (-P) : becomes a master as soon as possible
--addr=<ip> (-a <ip>) : virtual shared IP address
--help (-h) : summary of command-line options
--advbase=<seconds> (-b <seconds>) : advertisement frequency
--advskew=<skew> (-k <skew>) : advertisement skew (0-255)
--upscript=<file> (-u <file>) : run <file> to become a master
--downscript=<file> (-d <file>) : run <file> to become a backup
--deadratio=<ratio> (-r <ratio>) : ratio to consider a host as dead
--shutdown (-z) : call shutdown script at exit
--daemonize (-B) : run in background

Sample usage :

Manage the 10.1.1.252 shared virtual address on interface eth0, with
1 as a virtual address idenfitier, mypassword as a password, and
10.1.1.1 as a real permanent address for this host.
Call /etc/vip-up.sh when the host becomes a master, and
/etc/vip-down.sh when the virtual IP address has to be disabled.

ucarp --interface=eth0 --srcip=10.1.1.1 --vhid=1 --pass=mypassword \
      --addr=10.1.1.252 \
      --upscript=/etc/vip-up.sh --downscript=/etc/vip-down.sh


Please report bugs to  Bad IP checksum Bad TTL : [%u] Bad digest - md2=[%02x%02x%02x%02x...] md=[%02x%02x%02x%02x...] - Check vhid, password and virtual IP address Bad version : [%u] Dead ratio can't be zero Going back to BACKUP state Ignoring vhid : [%u] Interface name too long Invalid address : [%s] Invalid media / hardware address for [%s] Local advertised ethernet address is [%02x:%02x:%02x:%02x:%02x:%02x] Out of memory Out of memory to create packet Password too long Putting MASTER DOWN (going to time out) Putting MASTER down - preemption Spawning [%s %s] Switching to state : BACKUP Switching to state : INIT Switching to state : MASTER Unable to compile pcarp rule : %s [%s] Unable to detach : /dev/null can't be duplicated Unable to detach from the current session: %s Unable to find MAC address of [%s] Unable to get hardware info about an interface : %s Unable to get in background : [fork: %s] Unable to get interface address : %s Unable to open interface [%s] : %s Unable to open raw device : [%s] Unable to spawn the script : %s Unknown hardware type [%u] Unknown state : [%d] Using [%s] as a network interface Warning : no script called when going down Warning : no script called when going up You must supply a network interface You must supply a password You must supply a persistent source address You must supply a valid virtual host id You must supply a virtual host address You must supply an advertisement time base master_down event in INIT state out of memory to send gratuitous ARP write() error #%d/%d write() has failed Project-Id-Version: ucarp 1.0
Report-Msgid-Bugs-To: bugs@ucarp.org
POT-Creation-Date: 2004-06-20 21:37+0200
PO-Revision-Date: 2004-06-20 21:37+0200
Last-Translator: Automatically generated
Language-Team: none
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 8bit
Plural-Forms: nplurals=2; plural=(n != 1);
 * advertisement injected * --interface=<if> (-i <if>) : bind interface <if>
--srcip=<ip> (-s <ip>) : source (real) IP address of that host
--vhid=<id> (-v <id>) : virtual IP identifier (1-255)
--pass=<pass> (-p <pass>) : password
--preempt (-P) : becomes a master as soon as possible
--addr=<ip> (-a <ip>) : virtual shared IP address
--help (-h) : summary of command-line options
--advbase=<seconds> (-b <seconds>) : advertisement frequency
--advskew=<skew> (-k <skew>) : advertisement skew (0-255)
--upscript=<file> (-u <file>) : run <file> to become a master
--downscript=<file> (-d <file>) : run <file> to become a backup
--deadratio=<ratio> (-r <ratio>) : ratio to consider a host as dead
--shutdown (-z) : call shutdown script at exit
--daemonize (-B) : run in background

Sample usage :

Manage the 10.1.1.252 shared virtual address on interface eth0, with
1 as a virtual address idenfitier, mypassword as a password, and
10.1.1.1 as a real permanent address for this host.
Call /etc/vip-up.sh when the host becomes a master, and
/etc/vip-down.sh when the virtual IP address has to be disabled.

ucarp --interface=eth0 --srcip=10.1.1.1 --vhid=1 --pass=mypassword \
      --addr=10.1.1.252 \
      --upscript=/etc/vip-up.sh --downscript=/etc/vip-down.sh


Please report bugs to  Bad IP checksum Bad TTL : [%u] Bad digest - md2=[%02x%02x%02x%02x...] md=[%02x%02x%02x%02x...] - Check vhid, password and virtual IP address Bad version : [%u] Dead ratio can't be zero Going back to BACKUP state Ignoring vhid : [%u] Interface name too long Invalid address : [%s] Invalid media / hardware address for [%s] Local advertised ethernet address is [%02x:%02x:%02x:%02x:%02x:%02x] Out of memory Out of memory to create packet Password too long Putting MASTER DOWN (going to time out) Putting MASTER down - preemption Spawning [%s %s] Switching to state : BACKUP Switching to state : INIT Switching to state : MASTER Unable to compile pcarp rule : %s [%s] Unable to detach : /dev/null can't be duplicated Unable to detach from the current session: %s Unable to find MAC address of [%s] Unable to get hardware info about an interface : %s Unable to get in background : [fork: %s] Unable to get interface address : %s Unable to open interface [%s] : %s Unable to open raw device : [%s] Unable to spawn the script : %s Unknown hardware type [%u] Unknown state : [%d] Using [%s] as a network interface Warning : no script called when going down Warning : no script called when going up You must supply a network interface You must supply a password You must supply a persistent source address You must supply a valid virtual host id You must supply a virtual host address You must supply an advertisement time base master_down event in INIT state out of memory to send gratuitous ARP write() error #%d/%d write() has failed 