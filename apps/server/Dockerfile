# 构建阶段
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/catcube ./cmd/catcube

# 运行阶段
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/catcube /usr/local/bin/catcube
EXPOSE 8080
ENTRYPOINT ["catcube"]
