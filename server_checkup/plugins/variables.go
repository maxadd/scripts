package plugins

import (
	"fmt"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"os"
	"sync"
)

const (
	chanSize       = 100
	ConnectionKind = "connection"
)

var PluginMap map[string]func(*ssh.Session, *FailedMsg, string)

func registryPlugin(name string, plugin func(*ssh.Session, *FailedMsg, string)) {
	if PluginMap == nil {
		PluginMap = make(map[string]func(*ssh.Session, *FailedMsg, string))
	}
	PluginMap[name] = plugin
}

func init() {
	FailedMsgChan = make(chan *FailedMsg, chanSize)
	EndChan = make(chan struct{})
}

var FailedMsgChan chan *FailedMsg
var EndChan chan struct{}
var Wg sync.WaitGroup

type TomlConfig struct {
	Inputs         map[string]map[string]string
	Passwords      map[string]PasswordConfig
	Plugins        []PluginConfig
	ExcludeIPsFile string
	DingdingUrl    string
	CsvFilePath    string
}

type PasswordConfig struct {
	Password string
	Type     string
}

type PluginConfig struct {
	Kind       string
	User       string
	Cmd        string
	ExcludeIPs []string
}

type FailedMsgContent struct {
	Kind    string
	Content string
}

type FailedMsg struct {
	IP  string
	Msg []*FailedMsgContent
}

func Exit(code int) {
	glog.Flush()
	os.Exit(code)
}

func OutputErrorMsg(msg *FailedMsg, err interface{}, kind, cmd string) {
	failedMsg := fmt.Sprintf("%s %s checkup exception, failed to execute command %s: %v",
		msg.IP, kind, cmd, err)
	msg.Msg = append(msg.Msg, &FailedMsgContent{Kind: kind,
		Content: failedMsg})
	glog.Error(failedMsg)
}

func getRecover(msg *FailedMsg, kind, cmd string) {
	if err := recover(); err != nil {
		OutputErrorMsg(msg, err, kind, cmd)
	}
}
