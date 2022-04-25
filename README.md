# kvdb 介绍
基于 Raft 算法实现的 key/value 分布式存储系统。

# kvdb 安装
1.下载加载所需依赖.
```
go mod download
```
2.通过 docker 来模拟集群情况,需要执行 Dockerfile 构建镜像.
```
# 在 kvdb 目录下执行
docker build -t kvdb:v1.0 .
```

# kvdb 运行
运行 docker 镜像,在 Dockerfile 中,会调用 container.sh 脚本。该脚本会修改 kvdb 启动所需的一些配置信息(conf.json).
```
docker run -itd \
 --name node1 \
 --volume /Users/zhangkai/Documents/GoProject/kvdb/container.sh:/app/app_start.sh \
 kvdb:v1.0 /bin/sh -c 'sh /app/app_start.sh -id=1 -bp=true'
```
通过 sh 命令传递节点启动所需的参数.
```
-id :表示该节点 id,唯一
-bp :表示该节点是否初始化启动,决定了启动的集群个数(为 true 的节点是无法被 join 到其他的 raft 集群中的,因为在启动时已经选举出它的集群 leader)
```

1.启动容器作为集群的三个节点,其中只有 node1 bp=true,构成集群 A.
```
docker run -itd \
 --name node1 \
 --volume /Users/zhangkai/Documents/GoProject/kvdb/container.sh:/app/app_start.sh \
 kvdb:v1.0 /bin/sh -c 'sh /app/app_start.sh -id=1 -bp=true'
 
docker run -itd \
 --name node2 \
 --volume /Users/zhangkai/Documents/GoProject/kvdb/container.sh:/app/app_start.sh \
 kvdb:v1.0 /bin/sh -c 'sh /app/app_start.sh -id=2 -bp=false'

docker run -itd \
 --name node2 \
 --volume /Users/zhangkai/Documents/GoProject/kvdb/container.sh:/app/app_start.sh \
 kvdb:v1.0 /bin/sh -c 'sh /app/app_start.sh -id=3 -bp=false'
```
2.通过 API 将另外两个节点加入集群 A 中.
```
curl -X POST 172.17.0.2:8081/join -d '{"id":"2","httpAddr":"172.17.0.3:8081","raftAddr":"172.17.0.3:8089"}' -L
curl -X POST 172.17.0.2:8081/join -d '{"id":"3","httpAddr":"172.17.0.4:8081","raftAddr":"172.17.0.4:8089"}' -L
```
3.增加 key/value(因为是多节点,我们可以调用集群中任何节点的 ip 完成操作).
```
curl -X POST 172.17.0.2:8081/key -d '{"foo": "bar"}' -L
```
4.获取 key 对应的 value
```
curl -X GET 172.17.0.3:8081/key/foo -L
```
5.删除 key/value.
```
curl -X DELETE 172.17.0.3:8081/key/foo -L
```

# 你可能会遇到的问题
1.进入容器,vi conf.json 可以看到当前节点对外使用的 ip:port.

2.Mac 可能 ping 不通容器,可以参考: https://www.haoyizebo.com/posts/fd0b9bd8/
通过 docker-connector 来解决网桥问题.
