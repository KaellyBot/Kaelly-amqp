# Kaelly-amqp

[![Golangci-lint](https://github.com/kaellybot/kaelly-amqp/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/kaellybot/kaelly-amqp/actions/workflows/golangci-lint.yml)
[![Test](https://github.com/kaellybot/kaelly-amqp/actions/workflows/test.yml/badge.svg)](https://github.com/kaellybot/kaelly-amqp/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/kaellybot/kaelly-amqp/branch/main/graph/badge.svg)](https://codecov.io/gh/kaellybot/kaelly-amqp) 

Library to discuss with RabbitMQ via Protobuf, built in Go 

## Installation

This project uses the `protoc` command to generate Go files from proto files. 

- To install the protobuf compiler:
```
apt install protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

- To generate new go file based on proto source:
```
protoc --go_out=. rabbitmq.proto
```