package system

import (
	"os/exec"
	"runtime"
	"strings"
)

// RouteTable returns the kernel routing table as text.
func RouteTable(v6 bool) string {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "linux":
		fam := "-4"
		if v6 {
			fam = "-6"
		}
		out, err = exec.Command("ip", fam, "route", "show").CombinedOutput()
	default:
		fam := "inet"
		if v6 {
			fam = "inet6"
		}
		out, err = exec.Command("netstat", "-rn", "-f", fam).CombinedOutput()
	}
	if err != nil && len(out) == 0 {
		return "% " + err.Error() + "\n"
	}
	return string(out)
}

// Uname returns the operating system description.
func Uname() string {
	out, err := exec.Command("uname", "-a").Output()
	if err != nil {
		return runtime.GOOS
	}
	return strings.TrimSpace(string(out))
}

// Uptime returns the uptime(1) line.
func Uptime() string {
	out, err := exec.Command("uptime").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PingCommand returns the ping command for an address family.
func PingCommand(v6 bool, count int) (string, []string) {
	c := []string{"-c", itoa(count)}
	if v6 && runtime.GOOS == "netbsd" {
		return "ping6", c
	}
	if v6 {
		return "ping", append([]string{"-6"}, c...)
	}
	return "ping", c
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
