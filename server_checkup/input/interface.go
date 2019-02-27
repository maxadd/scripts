package input

import "server_checkup/plugins"

type Interface interface {
	GetIPList(configs *plugins.TomlConfig) map[string]struct{}
}
