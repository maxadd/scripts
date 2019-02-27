package main

import (
	"bufio"
	"encoding/base64"
	"flag"
	"fmt"
	"github.com/maxadd/glog"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"os"
	"runtime"
	"server_checkup/input"
	"server_checkup/plugins"
	"strings"
	"sync"
)

func getConfigFile() string {
	var file string
	flag.StringVar(&file, "f", "", "config file")
	flag.Parse()

	var t []string
	if file == "" {
		t = append(t, "-f")
	}

	if len(t) > 0 {
		fmt.Fprintf(os.Stderr, "Missing required options: %s\n", strings.Join(t, ", "))
		os.Exit(1)
	}
	return file
}

func sshConnection(user, ip, password, passwdKind string, msg *plugins.FailedMsg) *ssh.Client {
	config := getSshClientSession(user, password, passwdKind)
	client, err := ssh.Dial("tcp", ip+":22", config)
	if err != nil {
		glog.Error("Failed to dial: ", err)
		plugins.FailedMsgChan <- &plugins.FailedMsg{
			IP: ip,
			Msg: append(msg.Msg, &plugins.FailedMsgContent{
				Kind:    "connection",
				Content: ip + " connection failed, " + err.Error(),
			}),
		}
		runtime.Goexit()
	}
	return client
}

func getSessionFromConn(client *ssh.Client) *ssh.Session {
	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create session: ", err)
		runtime.Goexit()
	}
	return session
}

func publicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read ssh key file, ", err)
		plugins.Exit(1)
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse ssh key file, ", err)
		plugins.Exit(1)
	}
	return ssh.PublicKeys(key)
}

func getSshClientSession(user, password, passwdKind string) *ssh.ClientConfig {
	switch passwdKind {
	case "key":
		return &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				publicKeyFile(password),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
	case "password":
		return &ssh.ClientConfig{
			User: user,
			Auth: []ssh.AuthMethod{
				ssh.Password(base64decode(password)),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
	}
	fmt.Fprintln(os.Stderr, "Wrong configuration file, `passwords.USER.type` can only be key|password, not ", passwdKind)
	plugins.Exit(1)
	return nil
}

func excludeIPsFromFile(i input.Interface, fileName string, configs *plugins.TomlConfig) map[string]struct{} {
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open file failed, ", err)
		os.Exit(1)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	ips := i.GetIPList(configs)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		delete(ips, ip)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read file failed, ", err)
		os.Exit(2)
	}
	return ips
}

func connectAndExecCmd(configs *plugins.TomlConfig, ip string) {
	defer plugins.Wg.Done()
	sshClientMap := map[string]*ssh.Client{}
	failedMsg := &plugins.FailedMsg{IP: ip, Msg: []*plugins.FailedMsgContent{}}
	for _, v := range configs.Plugins {
		execCmd(configs, ip, failedMsg, sshClientMap, v)
	}
	for _, client := range sshClientMap {
		client.Close()
	}
	if len(failedMsg.Msg) > 0 {
		plugins.FailedMsgChan <- failedMsg
	}
}

func execCmd(configs *plugins.TomlConfig, ip string, failedMsg *plugins.FailedMsg,
	sshClientMap map[string]*ssh.Client, v plugins.PluginConfig) {
	for i := 0; i < len(v.ExcludeIPs); i++ {
		if v.ExcludeIPs[i] == ip {
			glog.Infof("exclude ip %s in %s", ip, v.Kind)
			return
		}
	}
	client, ok := sshClientMap[v.User]
	if !ok {
		client = sshConnection(
			v.User, ip,
			configs.Passwords[v.User].Password,
			configs.Passwords[v.User].Type,
			failedMsg)
		sshClientMap[v.User] = client
		//fmt.Println("create ssh connection")
	}
	session := getSessionFromConn(client)
	plugins.PluginMap[v.Kind](session, failedMsg, v.Cmd)
	session.Close()
}

func base64decode(src string) string {
	bytes, e := base64.StdEncoding.DecodeString(src)
	if e != nil {
		fmt.Fprintf(os.Stderr, "the password %s is not a valid base64 string\n", e)
		plugins.Exit(1)
	}
	return string(bytes)
}

func configValidate(configs *plugins.TomlConfig) {
	if len(configs.Inputs) > 1 {
		fmt.Fprintln(os.Stderr, "input can only have one, but now ", len(configs.Inputs))
		os.Exit(1)
	}
	configs.CsvFilePath = strings.TrimSuffix(configs.CsvFilePath, "/")
}

func main() {
	defer glog.Flush()
	configs := loadToml(getConfigFile())
	fmt.Println(configs)
	configValidate(configs)
	i := input.IpFromZabbix{}
	ips := excludeIPsFromFile(i, configs.ExcludeIPsFile, configs)
	go getFailedMsg(configs, ips)
	plugins.Wg = sync.WaitGroup{}

	for ip := range ips {
		plugins.Wg.Add(1)
		go connectAndExecCmd(configs, ip)
	}
	plugins.Wg.Wait()
	glog.Warning("all goroutines are executed!")
	close(plugins.FailedMsgChan)
	<-plugins.EndChan
}
