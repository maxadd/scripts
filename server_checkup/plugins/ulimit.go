package plugins

import (
	"bytes"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"strconv"
	"strings"
)

const (
	ulimitThreshold   = 65535
	ulimitCheckupName = "ulimit"
)

func init() {
	registryPlugin(ulimitCheckupName, ulimitCheckup)
}

func ulimitCheckup(session *ssh.Session, msg *FailedMsg, cmd string) {
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		OutputErrorMsg(msg, err, ulimitCheckupName, cmd)
		return
	}

	result := b.String()
	i, err := strconv.Atoi(strings.TrimSpace(result))
	if err != nil {
		OutputErrorMsg(msg, err, ulimitCheckupName, cmd)
		return
	}

	if i < ulimitThreshold {
		OutputErrorMsg(msg, i, ulimitCheckupName, cmd)
		return
	}
	glog.Infof("%s %s checkup succeeded!", msg.IP, ulimitCheckupName)
}
