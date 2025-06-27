#!/bin/bash

# Install jq
sudo apt update
sudo apt install -y jq

# Install grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
export PATH=$PATH:$(go env GOPATH)/bin
