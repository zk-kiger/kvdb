package main

import (
	"flag"
	"github.com/zk-kiger/raft"
	"kvdb/conf"
	"kvdb/server"
	"kvdb/store"
	"log"
	"os"
	"os/signal"
	"time"
)

const (
	Bootstrap  = false
	ConfigPath = "./config_single.json"
)

// command line parameters
var (
	bootstrap  bool
	configPath string
)

func init() {
	flag.BoolVar(&bootstrap, "bootstrap", Bootstrap, "bootstrap the raft node.")
	flag.StringVar(&configPath, "configPath", ConfigPath, "cluster node config info path.")
}

func main() {
	flag.Parse()

	// 存在未解析的参数.
	if flag.NArg() != 0 {
		log.Fatalf("failed to parse flag, because of unparsed args.")
	}

	log.Println("bootstrap", bootstrap)
	log.Println("configPath", configPath)

	config, err := conf.BootstrapConfig(configPath)
	if err != nil {
		log.Fatalf("bootstrap config, err: %v", err)
	}

	err = conf.ValidateConfig(config)
	if err != nil {
		log.Fatalf("failed to validate config, err: %v", err)
	}

	var s *store.KvStore
	if bootstrap {
		s, err = store.NewKvStore(config)
		if err != nil {
			log.Fatalf("failed to make bootstrap KvStore, err: %v", err)
		}

		// 确保集群进入一致性状态.
		longStopTimeout := 10 * time.Second
		if _, err = s.WaitForLeader(longStopTimeout); err != nil {
			log.Fatalf("failed to wait for leader, err: %v", err)
		}
		if err = s.WaitForApplied(longStopTimeout); err != nil {
			log.Fatalf("failed to wait for applied index, err: %v", err)
		}
	} else {
		s, err = store.NewKvStoreNoBootstrap(config)
		if err != nil {
			log.Fatalf("failed to make no bootstrap KvStore, err: %v", err)
		}
	}

	// 存储 meta info,如果非 bootstrap 启动,节点会以 follower 运行,
	// 即使失败也不会影响集群运行,后续节点加入到集群后,leader 会同步数据.
	if err = s.SetMeta(config.Local.Id, config.Local.HttpAddr); err != raft.ErrNotLeader && err != nil {
		log.Fatalf("failed to set local meta, err: %v", err)
	}
	for _, c := range config.Nodes {
		if err = s.SetMeta(c.Id, c.HttpAddr); err != raft.ErrNotLeader && err != nil {
			log.Fatalf("failed to set nodes meta, err: %v", err)
		}
	}

	// 开启 http 监听客户端操作.
	layer := server.NewHttpLayer(config.Local.HttpAddr, s)
	if err = layer.Start(); err != nil {
		log.Fatalf("failed to start HTTP service, err: %v", err)
	}

	log.Println("kvdb started successfully!")

	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, os.Interrupt)
	<-terminate
	log.Println("kvdb exiting")
}