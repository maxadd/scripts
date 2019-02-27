package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/maxadd/glog"
	"net/http"
	"os"
	"os/exec"
	"server_checkup/plugins"
	"time"
)

var m map[string]int
var failedMsgCount map[string]int
var allCheckupFailedIP map[string]struct{}

const timeFormat = "20060102-1504"
const fileReceiveIP = "10.2.1.1"
const fileReceiveDir = "xxx"
const fileReceivePath = "/usr/share/nginx/html/" + fileReceiveDir
const markdownMsgHead = `总共要巡检的机器 %d 台，其中 %d 台连接失败。

以下是连接成功机器中巡检出的异常：

---

`
const markdownMsgTail = "---\n\n点击[此处](http://" + fileReceiveIP + "/" + fileReceiveDir + "/%s)查看详细信息。"

// 除了将巡检结果发送到钉钉，还会将所有 ip 的巡检结果写入到 CVS 文件中
// 首先循环存放所有巡检异常的 channel，循环一次就获得一个错误消息
// 错误消息中包含 ip 以及巡检异常的巡检项和巡检异常的信息
// 每拿到一个错误消息就构成 CSV 文件的一行，并统计其中每个巡检项总的失败的次数，作为钉钉消息发送
// 有异常就将异常信息输出到 CSV 文件中的对应列，没有异常就输出 yes

type dingdingTextBody struct {
	Msgtype string `json:"msgtype"`
	Text    *text  `json:"text"`
}

type text struct {
	Content string `json:"content"`
}

func dingding(ipNum int, fileName, dingdingURL string) {
	var text string
	connFailedCount, _ := failedMsgCount[plugins.ConnectionKind]
	delete(m, plugins.ConnectionKind)
	// 这是统计错误巡检的次数，比如 ntp 多少台巡检失败，这也是钉钉消息的内容
	for kind := range m {
		count, ok := failedMsgCount[kind]
		if !ok {
			text += "- `" + kind + "`: 0\n"
			continue
		}
		text += fmt.Sprintf("- `%s`: %d\n", kind, count)
	}

	content := fmt.Sprintf(markdownMsgHead, ipNum, connFailedCount) + text + "\n" +
		fmt.Sprintf(markdownMsgTail, fileName)

	sendMsgToDingding(dingdingURL, getMarkdownMsg(content))
}

func getTextMsg(msg string) []byte {
	a := dingdingTextBody{
		"text",
		&text{
			Content: msg,
		},
	}

	dingdingMsg, e := json.Marshal(a)
	if e != nil {
		glog.Error("dingding message json.Marshal failed, ", e)
		plugins.Exit(1)
	}

	return dingdingMsg
}

func getFailedMsg(configs *plugins.TomlConfig, ips map[string]struct{}) {
	createMapping(configs)
	fileName := time.Now().Format(timeFormat) + ".csv"
	file, writer := createCSV(configs.CsvFilePath + "/" + fileName)
	genCSVHead(writer, len(configs.Plugins))
	defer end(configs, ips, writer, file, fileName)

	allCheckupFailedIP = make(map[string]struct{})
	for {
		msg, ok := <-plugins.FailedMsgChan
		if !ok {
			return
		}
		allCheckupFailedIP[msg.IP] = struct{}{}

		s := failedMsgProcess(msg, len(configs.Plugins))
		writeLine(writer, s)
	}

}

func end(configs *plugins.TomlConfig, ips map[string]struct{},
	writer *csv.Writer, file *os.File, fileName string) {
	defer close(plugins.EndChan)
	getAllCsvContent(ips, writer, len(configs.Plugins))
	csvFlush(writer)
	file.Close()
	sendCSVFile(configs.CsvFilePath, fileName)
	dingding(len(ips), fileName, configs.DingdingUrl)
}

func genCSVHead(writer *csv.Writer, size int) {
	s := make([]string, size+2)
	s[0] = "ip"
	for kind, idx := range m {
		s[idx] = kind
	}
	writeLine(writer, s)
}

func createMapping(configs *plugins.TomlConfig) {
	// 之所以 +1，是因为除了所有的巡检项之外，还有一个 connection 的异常，也就是连接不上的
	// CSV 文件中，第一列是 ip 地址，第二列是 plugins.ConnectionKind，从第三列开始才是一个个的巡检项
	m = make(map[string]int, len(configs.Plugins)+1)
	failedMsgCount = make(map[string]int, len(configs.Plugins)+1)
	m[plugins.ConnectionKind] = 1
	for i, v := range configs.Plugins {
		m[v.Kind] = i + 2
	}
	fmt.Println(m)
}

func sendCSVFile(csvFilePath, csvFileName string) {
	src, dest := csvFilePath+"/"+csvFileName, fileReceiveIP+":"+fileReceivePath+"/"
	fmt.Println("/usr/bin/scp", src, dest)
	cmd := exec.Command("/usr/bin/scp", src, dest)
	if err := cmd.Start(); err != nil {
		glog.Error("exec cmd failed, ", err)
		plugins.Exit(2)
	}
	if err := cmd.Wait(); err != nil {
		glog.Error("exec script failed,", err)
		plugins.Exit(1)
	}
}

func createCSV(filePath string) (*os.File, *csv.Writer) {
	//fmt.Println("open file", filePath)
	csvFile, err := os.Create(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open file %s failed, %s\n", filePath, err)
		plugins.Exit(1)
	}
	return csvFile, csv.NewWriter(csvFile)
}

// 这个函数用于构成 CSV 中的一行，当然这只是针对有巡检失败的机器
// 剩下的机器也要输入到 CSV 文件中，只是全部都是 yes，因为巡检都成功
// 只不过要在另一个函数中进行
func failedMsgProcess(msg *plugins.FailedMsg, size int) []string {
	s := make([]string, size+2)
	s[0] = msg.IP
	for _, v := range msg.Msg {
		s[m[v.Kind]] = v.Content
		failedMsgCount[v.Kind]++
	}

	for i, v := range s {
		if len(v) == 0 {
			s[i] = "yes"
		}
	}

	return s
}

// 剩下的巡检正常的机器都要写入到 CSV 文件中，只不过都为 yes
// size 表示所有巡检项的数量，它用于决定 CSV 文件的列数，一个巡检项就是一列
// 除了巡检项之外，还有两个固定列，一列是 ip 地址，一列是 ssh 连接是否失败
// 所以 CSV 文件的总列数为 size+2
func getAllCsvContent(ips map[string]struct{}, w *csv.Writer, size int) {
	for ip := range ips {
		_, ok := allCheckupFailedIP[ip]
		if ok {
			continue
		}
		s := make([]string, size+2)
		s[0] = ip
		for i := 1; i < size+2; i++ {
			s[i] = "yes"
		}
		writeLine(w, s)
	}
}

func csvFlush(w *csv.Writer) {
	w.Flush()
	err := w.Error()
	if err != nil {
		fmt.Fprintln(os.Stderr, "flush data into csv failed, ", err)
	}
}

func writeLine(w *csv.Writer, s []string) {
	err := w.Write(s)
	if err != nil {
		glog.Errorf("failed to write line %s to CSV file, %s", s, err)
	}
}

func sendMsgToDingding(url string, body []byte) {
	resp, err := http.Post(url,
		"application/json", bytes.NewReader(body))
	if err != nil {
		glog.Error("failed to send msg to dingding, ", err)
		plugins.Exit(1)
	}
	glog.Info(resp)
}

type dingdingMessageBody struct {
	Msgtype  string    `json:"msgtype"`
	Markdown *markdown `json:"markdown"`
}

type markdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

func getMarkdownMsg(msg string) []byte {
	a := dingdingMessageBody{
		"markdown",
		&markdown{
			Title: "搞事情",
			Text:  msg,
		},
	}

	dingdingMsg, e := json.Marshal(a)
	if e != nil {
		glog.Errorf("dingding message json.Marshal failed, %v", e)
		plugins.Exit(1)
	}

	return dingdingMsg
}
