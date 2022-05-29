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