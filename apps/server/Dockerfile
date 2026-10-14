# 构建阶段（依赖经 vendor/ 入库，构建无需访问模块代理）
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -o /out/catcube ./cmd/catcube

# 运行阶段（无包管理器；时区用代码内 FixedZone，不依赖 tzdata）
FROM gcr.io/distroless/static-debian12
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/catcube /usr/local/bin/catcube
EXPOSE 8080
ENTRYPOINT ["catcube"]
