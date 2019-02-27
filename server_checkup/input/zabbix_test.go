package input

import (
	"testing"
)

func TestIpFromZabbix_GetIPList(t *testing.T) {
	x := IpFromZabbix{}
	t.Log(x.GetIPList())
	//for _, v := range x.GetIPList() {
	//	t.Log(v)
	//}
}

func Test_getTemplateID(t *testing.T) {
	v := getTemplateID(getSessionID())
	//if v != "11858" {
	//	t.Error(v)
	//}
	t.Log(v)
}
