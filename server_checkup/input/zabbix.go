package input

import (
	"bytes"
	"fmt"
	"github.com/maxadd/glog"
	"github.com/tidwall/gjson"
	"io/ioutil"
	"net/http"
	"server_checkup/plugins"
)

const (
	zabbixServer   = "zabbix.uce.local"
	zabbixUser     = "monitor"
	zabbixPassword = "WSX@abc321,"
)

const body = `{
    "jsonrpc": "2.0",
    "method": "user.login",
    "params": {
        "user": "%s",
        "password": "%s"
    },
    "id": 1
}`

const getTemplateBody = `{
    "jsonrpc": "2.0",
    "method": "host.get",
    "params": {
        "output": ["host"],
        "templateids": "%s"
    },
    "auth": "%s",
    "id": 1
}`

const getTemplateIDBody = `{
    "jsonrpc": "2.0",
    "method": "template.get",
    "params": {
        "output": "templateids",
        "filter": {"host": ["%s"]}
    },
    "auth": "%s",
    "id": 1
}`

type IpFromZabbix map[string]struct{}

func (p IpFromZabbix) GetIPList(configs *plugins.TomlConfig) map[string]struct{} {
	sessionID := getSessionID()
	content := sendPostRequestToZabbix(
		[]byte(fmt.Sprintf(getTemplateBody, getTemplateID(sessionID,
			configs.Inputs["zabbix"]["template_name"]), sessionID)))
	result := gjson.GetBytes(content, "result")
	//fmt.Println(string(content))
	result.ForEach(p.iterateJson)
	return p
}

func sendPostRequestToZabbix(body []byte) []byte {
	resp, err := http.Post("http://"+zabbixServer+"/api_jsonrpc.php",
		"application/json",
		bytes.NewBuffer(body))
	if err != nil {
		glog.Fatal("connection to zabbix server failed, ", err)
	}
	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Fatal("failed to get response from zabbix api, ", err)
	}
	return content
}

func getTemplateID(sessionID, templateName string) string {
	content := sendPostRequestToZabbix(
		[]byte(fmt.Sprintf(getTemplateIDBody, templateName, sessionID)))
	result := gjson.GetBytes(content, "result.0.templateid")
	templateID := result.String()
	if len(templateID) == 0 {
		glog.Fatal("the template ID is not obtained. Please check the template name is correct. ", result)
	}
	return result.String()
}

func getSessionID() string {
	content := sendPostRequestToZabbix(
		[]byte(fmt.Sprintf(body, zabbixUser, zabbixPassword)))
	getBytes := gjson.GetBytes(content, "result")
	if len(getBytes.String()) == 0 {
		glog.Fatal("the response content of the zabbix api did not"+
			" find the relevant content of the sessionID. response content: ", content)
	}
	return getBytes.String()
}

func (p *IpFromZabbix) iterateJson(_, value gjson.Result) bool {
	(*p)[value.Get("host").String()] = struct{}{}
	//fmt.Println(p)
	return true
}
