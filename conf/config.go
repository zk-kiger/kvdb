package conf

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
)

const DefaultFilePath string = "./conf.json"

// Config 用于存储初始的 Raft 节点配置信息.
type Config struct {
	Local *RaftNode   `json:"local"`
	Nodes []*RaftNode `json:"nodes"`
}

type RaftNode struct {
	Id       string `json:"id"`
	HttpAddr string `json:"http_addr"`
	RaftAddr string `json:"raft_addr"`
	RaftDir  string `json:"raft_dir"`
}

func BootstrapConfig(filePath string) (*Config, error) {
	var (
		fl *os.File
		err error
	)
	if filePath == "" {
		fl, err = os.Open(DefaultFilePath)
	} else {
		fl, err = os.Open(filePath)
	}
	defer fl.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to open file, err: %v", err)
	}

	conf := &Config{}
	encoder := json.NewDecoder(fl)
	err = encoder.Decode(conf)
	if err != nil {
		return nil, fmt.Errorf("failed to decode json err: %v", err)
	}

	return conf, nil
}

func ValidateConfig(config *Config) error {
	if config.Local == nil {
		return fmt.Errorf("local raft node is empty")
	}

	var addr []string
	addr = append(addr, config.Local.HttpAddr)
	addr = append(addr, config.Local.RaftAddr)

	for _, node := range config.Nodes {
		addr = append(addr, node.HttpAddr)
		addr = append(addr, node.RaftAddr)
	}

	err := validateAddr(addr)
	if err != nil {
		return err
	}

	return nil
}

func validateAddr(addr []string) error {
	if len(addr) == 0 {
		return nil
	}

	for _, s := range addr {
		host, port, err := net.SplitHostPort(s)
		if err != nil {
			return err
		}

		ipv4 := net.ParseIP(host)
		if ipv4 == nil {
			return fmt.Errorf("invalid host name %s", host)
		}

		p, err := strconv.Atoi(port)
		if err != nil {
			return err
		}
		if p < 0 || p > 65535 {
			return fmt.Errorf("invalid port %d", p)
		}
	}

	return nil
}