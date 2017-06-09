How to get started using the ARM support on the Gateworks "Avila" board.
This procedure has been tested on a FreeBSD HEAD system.

1. Build world:

   make TARGET_ARCH=arm TARGET_CPUTYPE=xscale TARGET_BIG_ENDIAN=true buildworld

2. Setup an NFS root:

   ROOT=/data/freebsd/roots/gateworks
   make TARGET_ARCH=arm TARGET_CPUTYPE=xscale \
	TARGET_BIG_ENDIAN=true DESTDIR=$ROOT installworld
   mergemaster -m $SRC/etc -D $ROOT -i -A arm

   [Note mergemaster will fail if used to install HEAD on a RELENG_6
    system unless /usr/share/mk on the host system has been updated
    w/ changes to the build infrastructure.]

3. Configure the NFS root for diskless use; consult diskless(8).

4. Build and install a kernel:

   Load the npe(4) microcode file IxNpeMicrocode.dat from
	http://www.intel.com/design/network/products/npfamily/ixp400_current.htm
   and put it into /sys/arm/xscale/ixp425/

   make TARGET_ARCH=arm KERNCONF=AVILA buildkernel 
   make TARGET_ARCH=arm KERNCONF=AVILA DESTDIR=$ROOT installkernel 

   Note: the AVILA kernel is configured with the root filesystem
         mounted via NFS over the npe0 wired interface.

   You should now have an NFS-mountable root filesystem with a kernel.
   The final step is to setup network diskless booting from the board.
   It is assumed you have a DHCP server operating on your network and
   the server is configured to supply the necessary information in the
   DHCP lease.  If you run the ISC DHCP server the following configuration
   information is an example of how to do this:

   option	root-opts code 130 = string;	# NFS / mount options

   host avila1 {
       hardware ethernet 00:d0:12:02:47:68;
       fixed-address	10.0.0.221;
       next-server	10.0.0.251;
       filename		"kernel-avila.nfs";
       option root-path	"10.0.0.251:/data/freebsd/roots/gateworks";
       option root-opts	"nolockd";
   }

   [Note: the root-opts item specifies the root filesystem should
    be mounted with the nolocked option; this just short-circuits
    file locking requests so you don't get complaints from programs
    that use the pidfile(3) routine (e.g. devd).]

5. Place the kernel in the TFTP area for booting from the prom monitor.
   If your TFTP server returns file from /tftpboot (default) then do
   something like:

   cp $ROOT/boot/kernel/kernel /tftpboot/kernel-avila.nfs

6. Boot the kernel from redboot:

   RedBoot> ip -h 10.0.0.251 -l 10.0.0.1
   IP: 10.0.0.1/255.255.255.0 Gateway: 0.0.0.0
   Default server: 10.0.0.251
   RedBoot> load -b 0x200000 kernel-avila.nfs
   Using default protocol (TFTP)
   Address offset = 0x40000000
   Entry point: 0x00200100, address range: 0x00200000-0x004db2d4
   RedBoot> go

7. To record the above redboot script so that it's used every time
   the board is reset run the fconfig command at the redboot prompt
   and answer the questions.

Past this you can setup a FreeBSD installation on compact flash and
store a kernel in flash using redboot.  Then on boot load the kernel
from flash and if that kernel is configured with root on ad0 you
can run entirely from local storage.  Note that the redboot that comes
in the Avila board does not understand how to load a file from UFS
so it is not yet possible to boot a kernel directly from compact flash
(when formatted with UFS).

8. Boot kernel from CF:

   On Avila:
   Insert CF into the board
   Boot from NFS
   fdisk ad0 and bsdlabel ad0s1 (remind that xscale-arm.kernel is big-endian!)
   mount /dev/ad0s1a /mnt
   cd /; find -x . | cpio -pdamuv /mnt

   On the NFS Host:
   Adapt /sys/arm/conf/AVILA, comment out options NFS_ROOT, BOOTP*
   add option ROOTDEVNAME=\"ufs:ad0s1\"
   make kernel like 4.
   install kernel into tftpd area as kernel.ad0
   
   On Avila:
   reboot
   CTRL-C before kernel loading (you need to config your cua*0 as 3wire
     via /etc/rc.d/serial to let CTRL-C work) 
   load -b 0x200000 kernel.ad0
   fis create kernel.ad0 (no need to overwrite the linux-stuff)
   fconfig (to adapt the new kernel)
