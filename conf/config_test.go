package conf

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"testing"
)

func TestConfig_ValidateAddr(t *testing.T) {
	raftAddr := "localhost:8089"
	host, _, err := net.SplitHostPort(raftAddr)
	if err != nil {
		t.Fatalf("failed to split %s host port, err: %v", raftAddr, err)
	}
	ipv4 := net.ParseIP(host)
	if ipv4 != nil {
		// localhost 不能通过校验,本机使用 127.0.0.1
		t.Fatalf("failed to parse %s", host)
	}

	raftAddr2 := "127.0.0.1:8089"
	host, _, err = net.SplitHostPort(raftAddr2)
	if err != nil {
		t.Fatalf("failed to split %s host port, err: %v", raftAddr2, err)
	}
	ipv4 = net.ParseIP(host)
	if ipv4 == nil {
		t.Fatalf("failed to parse %s", host)
	}

	raftAddr3 := "raft-cluster-host-1:8089"
	host, _, err = net.SplitHostPort(raftAddr3)
	if err != nil {
		t.Fatalf("failed to split %s host port, err: %v", raftAddr3, err)
	}
	ipv4 = net.ParseIP(host)
	if ipv4 != nil {
		t.Fatalf("failed to parse %s", host)
	}
}

func TestConfig_DecodeConfJson(t *testing.T) {
	filePath := "../conf.json"
	fl, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("failed to open file, err: %v", err)
	}

	conf := &Config{}
	encoder := json.NewDecoder(fl)
	err = encoder.Decode(conf)
	if err != nil {
		log.Fatalf("failed to decode json, err: %v", err)
	}
}

