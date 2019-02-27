package plugins

import (
	"bytes"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"regexp"
	"strings"
)

const (
	dnsServer1     = ""
	dnsServer2     = ""
	linePrefix     = "nameserver"
	dnsCheckupName = "dns"
	regexpString   = `\s+`
)

var re *regexp.Regexp

func init() {
	registryPlugin(dnsCheckupName, dnsCheckup)
	re = regexp.MustCompile(regexpString)
}

func dnsCheckup(session *ssh.Session, msg *FailedMsg, cmd string) {
	defer getRecover(msg, dnsCheckupName, cmd)
	var b bytes.Buffer
	var count int
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		OutputErrorMsg(msg, err, dnsCheckupName, cmd)
		return
	}

	for _, v := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(v, linePrefix) {
			dnsIP := re.Split(v, 2)[1]
			if dnsIP == dnsServer1 || dnsIP == dnsServer2 {
				count++
			}
		}
	}
	if count == 0 {
		OutputErrorMsg(msg, "dns file format error", dnsCheckupName, cmd)
		return
	}
	glog.Infof("%s %s checkup succeeded!", msg.IP, dnsCheckupName)
}
