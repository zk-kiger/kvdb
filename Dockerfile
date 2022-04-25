# 多阶段构建,最大的使用场景就是将编译环境和运行环境分离.

# 基础镜像 golang 是非常庞大的,因为其中包含了所有的 Go 语言编译工具和库,
# 而运行时候我们仅仅需要编译后的程序就行了,不需要编译时的编译工具,最后生成的大体积镜像就是一种浪费.
FROM golang:alpine AS builder

# 移动到工作目录,该目录用于编译 GO 程序.
WORKDIR /build

# 设置必要的环境变量.
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY="https://goproxy.cn,direct"

# 将 gomoudle 文件复制到容器,并下载所需依赖.
COPY go.mod .
COPY go.sum .
RUN go mod download

# 将代码复制到容器.
COPY . .
# 编译成二进制可执行文件.
RUN go build -a -o kvdb .

# 该阶段用于构建 GO 程序运行镜像.
FROM alpine AS final

# 移动到工作目录,该目录用于运行 GO 程序.
WORKDIR /app

# 复制 builder 阶段相关产物(二进制执行文件..)到新的工作目录.
COPY --from=builder /build/kvdb /app/
COPY --from=builder /build/conf.json /app/

# 在容器中创建脚本.
RUN touch /app/app_start.sh
RUN echo "#!/bin/sh" > /app/app_start.sh
RUN echo "echo starting kvdb..." >> /app/app_start.sh
RUN chmod a+x /app/app_start.sh
ENV APP_FILE /app/app_start.sh

# 暴露服务端口.
EXPOSE 8081 8089

# 启动该镜像容器,需要在容器中执行脚本 container.sh.
#docker run -itd \
# --name node1 \
# --volume /Users/zhangkai/Documents/GoProject/kvdb/container.sh:/app/app_start.sh \
# kvdb:v1.0 /bin/sh -c 'sh /app/app_start.sh -id=1 -bp=true'