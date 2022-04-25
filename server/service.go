package server

import (
	"encoding/json"
	"fmt"
	"github.com/zk-kiger/raft"
	"io"
	"kvdb/conf"
	"kvdb/store"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

type HttpLayer struct {
	listener net.Listener
	s        store.Store
	addr     string

	logger *log.Logger
}

func NewHttpLayer(addr string, s store.Store) *HttpLayer {
	return &HttpLayer{
		s:      s,
		addr:   addr,
		logger: log.New(os.Stderr, "[HTTPLayer]", log.LstdFlags),
	}
}

func (h *HttpLayer) FormRedirect(r *http.Request, host string) string {
	protocol := "http"
	rq := r.URL.RawQuery
	if rq != "" {
		rq = fmt.Sprintf("?%s", rq)
	}
	return fmt.Sprintf("%s://%s%s%s", protocol, host, r.URL.Path, rq)
}

// Start 开启 HTTP 服务.
func (h *HttpLayer) Start() error {
	server := http.Server{Addr: h.addr, Handler: h}

	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		return err
	}
	h.listener = ln

	http.Handle("/", h)

	go func() {
		err = server.Serve(h.listener)
		if err != nil {
			h.logger.Fatalf("failed to HTTP Serve: %v", err)
		}
	}()

	return nil
}

func (h *HttpLayer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/key") {
		h.handleKeyRequest(w, r)
	} else if r.URL.Path == "/join" {
		h.handleJoin(w, r)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

// Close 实现 Close 接口.
func (h *HttpLayer) Close() (err error) {
	return h.listener.Close()
}

func (h *HttpLayer) handleKeyRequest(w http.ResponseWriter, r *http.Request) {
	getKey := func() string {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return ""
		}
		return parts[2]
	}

	switch r.Method {
	case "GET":
		k := getKey()
		if k == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		lvl, err := level(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		val, err := h.s.Get(k, lvl)
		if err != nil {
			if err == raft.ErrNotLeader {
				leader := h.s.LeaderHttpAddr()
				if leader == "" {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				redirect := h.FormRedirect(r, leader)
				http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := json.Marshal(map[string]string{k: val})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if _, err = io.WriteString(w, string(b)); err != nil {
			h.logger.Printf("failed to WriteString: %v", err)
		}

	case "POST":
		// 从 request body 读取存储键值对.
		m := map[string]string{}
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for k, v := range m {
			if err := h.s.Set(k, v); err != nil {
				if err == raft.ErrNotLeader {
					leader := h.s.LeaderHttpAddr()
					if leader == "" {
						http.Error(w, err.Error(), http.StatusServiceUnavailable)
						return
					}

					redirect := h.FormRedirect(r, leader)
					http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
					return
				}

				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

	case "DELETE":
		k := getKey()
		if k == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := h.s.Delete(k); err != nil {
			if err == raft.ErrNotLeader {
				leader := h.s.LeaderHttpAddr()
				if leader == "" {
					http.Error(w, err.Error(), http.StatusServiceUnavailable)
					return
				}
				redirect := h.FormRedirect(r, leader)
				http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func level(r *http.Request) (store.ConsistencyLevel, error) {
	q := r.URL.Query()
	lvl := strings.TrimSpace(q.Get("level"))

	switch strings.ToLower(lvl) {
	case "default":
		return store.DEFAULT, nil
	case "stale":
		return store.STALE, nil
	case "consistent":
		return store.CONSISTENT, nil
	default:
		return store.DEFAULT, nil
	}
}

func (h *HttpLayer) handleJoin(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(m) != 3 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	httpAddr, ok := m["httpAddr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	raftAddr, ok := m["raftAddr"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	nodeID, ok := m["id"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	c := conf.RaftNode{
		Id: nodeID,
		HttpAddr: httpAddr,
		RaftAddr: raftAddr,
	}
	if err := h.s.Join(c); err != nil {
		if err == raft.ErrNotLeader {
			leader := h.s.LeaderHttpAddr()
			if leader == "" {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			redirect := h.FormRedirect(r, leader)
			http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
