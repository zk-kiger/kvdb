#!/bin/sh

# 获取当前主机 ip.
ip=`ifconfig -a|grep inet|grep -v 127.0.0.1|grep -v inet6|awk '{print $2}'|tr -d "addr:"`

# 获取 raft-id,bootstrap 参数.
# -id=1 -bp=true
p1="$1"
p2="$2"
raft_id=${p1#*=}
bp=${p2#*=}

# 替换 conf.json 中 raft-cluster-host 为 ip.
sed -i "s/raft-cluster-host/${ip}/g" conf.json

# 替换 conf.json 中 raft-id 为指定 id.
sed -i "s/raft-id/${raft_id}/g" conf.json

# 执行 Go 程序.
./kvdb -bootstrap=${bp} -configPath=./conf.json