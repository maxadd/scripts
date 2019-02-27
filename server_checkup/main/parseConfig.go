package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"io/ioutil"
	"os"
	"server_checkup/plugins"
)

func loadToml(tomlFile string) *plugins.TomlConfig {
	b, err := ioutil.ReadFile(tomlFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open toml %s failed, %v\n", tomlFile, err)
		os.Exit(1)
	}

	var configs plugins.TomlConfig
	_, err = toml.Decode(string(b), &configs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse toml %s failed, %v\n", tomlFile, err)
		os.Exit(1)
	}
	return &configs
}
