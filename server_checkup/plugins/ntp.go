package plugins

import (
	"bytes"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"math"
	"strconv"
	"strings"
)

const (
	ntpThreshold   = 2
	ntpCheckupName = "ntp"
)

func init() {
	registryPlugin(ntpCheckupName, ntpCheckup)
}

func ntpCheckup(session *ssh.Session, msg *FailedMsg, cmd string) {
	defer getRecover(msg, ntpCheckupName, cmd)
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		OutputErrorMsg(msg, err, ntpCheckupName, cmd)
		return
	}

	s := strings.Split(strings.Split(b.String(), "\n")[1], " ")
	offset := s[len(s)-2]
	offset64, err := strconv.ParseFloat(offset, 64)
	if err != nil {
		OutputErrorMsg(msg, err, ntpCheckupName, cmd)
		return
	}

	if math.Abs(offset64) > ntpThreshold {
		OutputErrorMsg(msg, offset, ntpCheckupName, cmd)
		return
	}
	glog.Infof("%s %s check succeeded!", msg.IP, ntpCheckupName)
}
