package store

import (
	"kvdb/conf"
	"os"
	"testing"
	"time"
)

func inmemConfigSingleNode(t *testing.T) *conf.Config {
	cf := &conf.Config{
		Local: &conf.RaftNode{
			Id:       "1",
			HttpAddr: "127.0.0.1:8081",
			RaftAddr: "127.0.0.1:8089",
			RaftDir:  "$KVDBPATH",
		},
	}
	return cf
}

func TestStore_NewStoreSingleNode(t *testing.T) {
	config := inmemConfigSingleNode(t)
	defer os.RemoveAll(config.Local.RaftDir)

	_, err := NewKvStore(config)
	if err != nil {
		t.Fatalf("failed to make kv store, err: %v", err)
	}
}

func TestStore_StoreInmemSingleNode(t *testing.T) {
	config := inmemConfigSingleNode(t)
	defer os.RemoveAll(config.Local.RaftDir)

	s, err := NewKvStore(config)
	if err != nil {
		t.Fatalf("failed to make kv store, err: %v", err)
	}

	_, err = s.WaitForLeader(3 * time.Second)
	if err != nil {
		t.Fatalf("failed to wait for leader, err: %v", err)
	}

	err = s.Set("testK", "testV")
	if err != nil {
		t.Fatalf("failed to set kv, err: %v", err)
	}

	val, err := s.Get("testK", DEFAULT)
	if err != nil || val != "testV" {
		t.Fatalf("failed to get the value corresponding to %s", "testK")
	}
}
