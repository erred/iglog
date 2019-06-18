package main

//go:generate protoc -I:iglog iglog/iglog.proto --go_out=plugins=grpc:iglog
