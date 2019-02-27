package plugins

import (
	"bytes"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"strconv"
	"strings"
)

const (
	zabbixThreshold   = 3
	zabbixCheckupName = "zabbix_agent"
)

func init() {
	registryPlugin(zabbixCheckupName, zabbixCheckup)
}

func zabbixCheckup(session *ssh.Session, msg *FailedMsg, cmd string) {
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(cmd); err != nil {
		OutputErrorMsg(msg, err, zabbixCheckupName, cmd)
		return
	}

	result := b.String()
	i, err := strconv.Atoi(strings.TrimSpace(result))
	if err != nil {
		OutputErrorMsg(msg, err, zabbixCheckupName, cmd)
		return
	}

	if i < zabbixThreshold {
		OutputErrorMsg(msg, i, zabbixCheckupName, cmd)
		return
	}
	glog.Infof("%s %s checkup succeeded!", msg.IP, zabbixCheckupName)
}
