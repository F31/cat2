# 构建阶段（依赖经 vendor/ 入库，构建无需访问模块代理）
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o /out/catcube ./cmd/catcube && mkdir -p /out/tmp && chmod 1777 /out/tmp

# 运行阶段：scratch（静态二进制 + CA 证书 + 可写 /tmp），避免依赖 gcr.io 镜像源
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/tmp /tmp
COPY --from=build /out/catcube /usr/local/bin/catcube
WORKDIR /repo/apps/server
ENV HOME=/tmp
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["catcube"]
