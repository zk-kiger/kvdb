package store

import (
	"encoding/json"
	"fmt"
	"github.com/zk-kiger/raft"
	"io"
	"kvdb/conf"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	RetainSnapshotCount = 3
	RaftTimeout         = 5 * time.Second
	LeaderWaitDelay     = 100 * time.Millisecond
	AppliedWaitDelay    = 100 * time.Millisecond
)

type ConsistencyLevel int

const (
	DEFAULT ConsistencyLevel = iota
	STALE
	CONSISTENT
)

type command struct {
	Op    string `json:"op,omitempty"`
	Key   string `json:"key,omitempty"`
	Value string `json:"value,omitempty"`
}

type Store interface {
	Get(key string, mode ConsistencyLevel) (string, error)

	Set(key, value string) error

	Delete(key string) error

	Join(config conf.RaftNode) error

	LeaderHttpAddr() string

	SetMeta(key, value string) error
}

type KvStore struct {
	lock           sync.RWMutex
	m              map[string]string
	configurations []raft.Configuration

	raft   *raft.Raft
	logger *log.Logger

	RaftDir  string
	RaftAddr string
}

type MakeStoreOps struct {
	Config         *conf.Config
	Bootstrap      bool
	ConfigStoreFSM bool
	MakeFSMFunc    func() raft.FSM
}

func NewKvStore(config *conf.Config) (*KvStore, error) {
	return newKvStore(&MakeStoreOps{
		Config:    config,
		Bootstrap: true,
	})
}

func NewKvStoreNoBootstrap(config *conf.Config) (*KvStore, error) {
	return newKvStore(&MakeStoreOps{
		Config:    config,
		Bootstrap: false,
	})
}

func newKvStore(opts *MakeStoreOps) (*KvStore, error) {
	cf := opts.Config
	s := &KvStore{
		m:        make(map[string]string),
		logger:   log.New(os.Stderr, "[store] ", log.LstdFlags),
		RaftDir:  cf.Local.RaftDir,
		RaftAddr: cf.Local.RaftAddr,
	}

	raftConf := raft.DefaultConfig()
	raftConf.LocalID = raft.ServerID(cf.Local.Id)
	raftConf.LogLevel = "Warn"

	addr, err := net.ResolveTCPAddr("tcp", s.RaftAddr)
	if err != nil {
		return s, err
	}
	transport, err := raft.NewTCPTransport(s.RaftAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return s, fmt.Errorf("failed to make transport: %v", err)
	}

	snap, err := raft.NewFileSnapshotStore(s.RaftDir, RetainSnapshotCount, os.Stderr)
	if err != nil {
		return s, fmt.Errorf("failed to make snapshot store: %v", err)
	}

	var logStore raft.LogStore
	var stableStore raft.StableStore
	boltDB, err := raft.NewBoltStore(filepath.Join(s.RaftDir, "raft.db"))
	if err != nil {
		return s, fmt.Errorf("failed to make bolt store: %v", err)
	}
	logStore = boltDB
	stableStore = boltDB

	var fsm raft.FSM
	if opts.ConfigStoreFSM {
		fsm = &fsmConfigStore{
			FSM: (*kvFSM)(s),
		}
	} else {
		if opts.MakeFSMFunc != nil {
			fsm = opts.MakeFSMFunc()
		}
		fsm = (*kvFSM)(s)
	}

	r, err := raft.NewRaft(raftConf, fsm, logStore, stableStore, snap, transport)
	if err != nil {
		return s, fmt.Errorf("failed to make raft: %v", err)
	}
	s.raft = r

	if opts.Bootstrap {
		var configuration raft.Configuration
		configuration.Servers = append(configuration.Servers, raft.Server{
			ID:      raft.ServerID(cf.Local.Id),
			Address: raft.ServerAddress(cf.Local.RaftAddr),
		})

		for _, c := range cf.Nodes {
			server := raft.Server{
				ID:      raft.ServerID(c.Id),
				Address: raft.ServerAddress(c.RaftAddr),
			}
			configuration.Servers = append(configuration.Servers, server)
		}

		r.BootstrapCluster(configuration)
	}

	return s, nil
}

func (s *KvStore) WaitForLeader(timeout time.Duration) (string, error) {
	ticker := time.NewTicker(LeaderWaitDelay)
	defer ticker.Stop()
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}

	s.logger.Printf("waiting for leader up to %s", timeout)
	for {
		select {
		case <-ticker.C:
			addr := s.LeaderAddr()
			if addr != "" {
				return addr, nil
			}
		case <-timer:
			return "", fmt.Errorf("timeout expired")
		}
	}
}

func (s *KvStore) WaitForApplied(timeout time.Duration) error {
	if timeout == 0 {
		return nil
	}
	s.logger.Printf("waiting for up to %s for application of initial logs", timeout)
	if err := s.WaitForAppliedIndex(s.raft.LastIndex(), timeout); err != nil {
		return fmt.Errorf("timeout waiting for initial logs application")
	}
	return nil
}

func (s *KvStore) WaitForAppliedIndex(idx uint64, timeout time.Duration) error {
	ticker := time.NewTicker(AppliedWaitDelay)
	defer ticker.Stop()
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}

	for {
		select {
		case <-ticker.C:
			if s.raft.AppliedIndex() >= idx {
				return nil
			}
		case <-timer:
			return fmt.Errorf("timeout expired")
		}
	}
}

func (s *KvStore) LeaderAddr() string {
	return string(s.raft.Leader())
}

func (s *KvStore) LeaderID() (string, error) {
	leaderAddr := s.LeaderAddr()
	configurationFu := s.raft.GetConfiguration()
	if err := configurationFu.Error(); err != nil {
		s.logger.Fatalf("failed to get raft configuration, err: %v", err)
		return "", err
	}
	configuration := configurationFu.Configuration()
	for _, server := range configuration.Servers {
		if server.Address == raft.ServerAddress(leaderAddr) {
			return string(server.ID), nil
		}
	}
	return "", nil
}

func (s *KvStore) LeaderHttpAddr() string {
	id, err := s.LeaderID()
	if err != nil {
		return ""
	}

	addr, err := s.GetMeta(id)
	if err != nil {
		return ""
	}
	return addr
}

func (s *KvStore) Get(key string, mode ConsistencyLevel) (string, error) {
	if mode != STALE {
		if s.raft.State() != raft.Leader {
			return "", raft.ErrNotLeader
		}
	}

	if mode == CONSISTENT {
		if err := s.consistentRead(); err != nil {
			return "", err
		}
	}

	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.m[key], nil
}

func (s *KvStore) consistentRead() error {
	verifyFu := s.raft.VerifyLeader()
	if err := verifyFu.Error(); err != nil {
		return err
	}
	return nil
}

func (s *KvStore) Set(key, value string) error {
	if s.raft.State() != raft.Leader {
		return raft.ErrNotLeader
	}

	c := command{
		Op:    "set",
		Key:   key,
		Value: value,
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	future := s.raft.Apply(b, RaftTimeout)
	return future.Error()
}

func (s *KvStore) Delete(key string) error {
	if s.raft.State() != raft.Leader {
		return raft.ErrNotLeader
	}

	c := &command{
		Op:  "delete",
		Key: key,
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}

	f := s.raft.Apply(b, RaftTimeout)
	return f.Error()
}

func (s *KvStore) Join(config conf.RaftNode) error {
	nodeId := config.Id
	httpAddr := config.HttpAddr
	raftAddr := config.RaftAddr
	s.logger.Printf("received join request for remote node %s at %s", nodeId, raftAddr)

	configurationFu := s.raft.GetConfiguration()
	if err := configurationFu.Error(); err != nil {
		s.logger.Printf("failed to get raft configuration: %v", err)
		return err
	}

	for _, svr := range configurationFu.Configuration().Servers {
		if svr.ID == raft.ServerID(nodeId) || svr.Address == raft.ServerAddress(raftAddr) {
			// raft 集群配置已经存在,无需添加.
			if svr.ID == raft.ServerID(nodeId) && svr.Address == raft.ServerAddress(raftAddr) {
				s.logger.Printf("node %s at %s already member of cluster, ignoring join request", nodeId, raftAddr)
				return nil
			}
		}
	}

	// 添加配置,如果 raft 配置中存在该配置,raft 会覆盖旧的地址.
	future := s.raft.AddVoter(raft.ServerID(nodeId), raft.ServerAddress(raftAddr), 0, 0)
	if err := future.Error(); err != nil {
		return err
	}

	if err := s.SetMeta(nodeId, httpAddr); err != nil {
		return err
	}

	s.logger.Printf("node %s at %s joined successfully", nodeId, raftAddr)
	return nil
}

func (s *KvStore) SetMeta(key, value string) error {
	return s.Set(key, value)
}

func (s *KvStore) GetMeta(key string) (string, error) {
	return s.Get(key, STALE)
}

type kvFSM KvStore

func (f *kvFSM) Apply(log *raft.Log) interface{} {
	var c command
	if err := json.Unmarshal(log.Data, &c); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}

	switch c.Op {
	case "set":
		return f.applySet(c.Key, c.Value)
	case "delete":
		return f.applyDelete(c.Key)
	default:
		panic(fmt.Sprintf("unrecognized command op: %s", c.Op))
	}
}

func (f *kvFSM) applySet(key string, value string) interface{} {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.m[key] = value
	return nil
}

func (f *kvFSM) applyDelete(key string) interface{} {
	f.lock.Lock()
	defer f.lock.Unlock()
	delete(f.m, key)
	return nil
}

func (f *kvFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	// Clone the map.
	o := make(map[string]string)
	for k, v := range f.m {
		o[k] = v
	}
	return &fsmSnapshot{kv: o}, nil
}

func (f *kvFSM) Restore(inp io.ReadCloser) error {
	o := make(map[string]string)
	if err := json.NewDecoder(inp).Decode(&o); err != nil {
		return err
	}

	f.m = o
	return nil
}

type fsmSnapshot struct {
	kv map[string]string
}

func (m *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		b, err := json.Marshal(m.kv)
		if err != nil {
			return err
		}

		if _, err = sink.Write(b); err != nil {
			return err
		}

		return sink.Close()
	}()

	if err != nil {
		err = sink.Cancel()
		return err
	}

	return nil
}

func (m *fsmSnapshot) Release() {}

type fsmConfigStore struct {
	raft.FSM
}

func (c *fsmConfigStore) StoreConfiguration(index uint64, config raft.Configuration) {
	mm := c.FSM.(*kvFSM)
	mm.lock.Lock()
	defer mm.lock.Unlock()
	mm.configurations = append(mm.configurations, config)
}