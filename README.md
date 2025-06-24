# distninja

[![Build Status](https://github.com/distninja/distninja/workflows/ci/badge.svg?branch=main&event=push)](https://github.com/distninja/distninja/actions?query=workflow%3Aci)
[![Go Report Card](https://goreportcard.com/badge/github.com/distninja/distninja)](https://goreportcard.com/report/github.com/distninja/distninja)
[![License](https://img.shields.io/github/license/distninja/distninja.svg)](https://github.com/distninja/distninja/blob/main/LICENSE)
[![Tag](https://img.shields.io/github/tag/distninja/distninja.svg)](https://github.com/distninja/distninja/tags)



## Introduction

distninja is a distributed build system



## Features

- **Graph Database Power** - Uses [cayley](https://github.com/distninja/cayley)'s quad-based storage for complex relationships
- **Schema Support** - Structured data with Go struct mapping
- **Rich Queries** - Path-based queries for complex dependency analysis
- **Relationship Modeling** - Explicit modeling of all Ninja relationships
- **Cycle Detection** - Built-in circular dependency detection
- **Performance** - Efficient graph traversal and querying



## Usage

### 1. HTTP Server

```bash
# Deploy server
distninja serve --http <string> --store <string>
```
```bash
# Test server
go run main.go serve --http :9090 --store /tmp/ninja.db
./script/http.sh
```

### 2. gRPC Server

```bash
# Deploy server
distninja serve --grpc <string> --store <string>
```

```bash
# Test server
go run main.go serve --grpc :9090 --store /tmp/ninja.db
./script/grpc.sh
```



## APIs

### 1. HTTP APIs

- **Build APIs**
  - `POST /api/v1/builds` - Create new build
  - `GET /api/v1/builds/{id}` - Get specific build
  - `GET /api/v1/builds/stats` - Get build statistics
  - `GET /api/v1/builds/order` - Get topological build order


- **Rule APIs**
  - `POST /api/v1/rules` - Create new rule
  - `GET /api/v1/rules/{name}` - Get specific rule
  - `GET /api/v1/rules/{name}/targets` - Get targets using a rule


- **Target APIs**
  - `GET /api/v1/targets` - Get all targets
  - `GET /api/v1/targets/{path}` - Get specific target
  - `GET /api/v1/targets/{path}/dependencies` - Get target dependencies
  - `GET /api/v1/targets/{path}/dependents` - Get reverse dependencies
  - `PUT /api/v1/targets/{path}/status` - Update target status


- **Analysis APIs**
  - `GET /api/v1/analysis/cycles` - Find circular dependencies


- **Debug APIs**
  - `GET /api/v1/debug/quads` - Debug quad information

### 2. gRPC APIs

*TBD*



## License

Project License can be found [here](LICENSE).



## Reference

- [cayley](https://github.com/distninja/cayley)
- [ninja](https://github.com/ninja-build/ninja)
- [ninja-build](https://gist.github.com/craftslab/a9cacfa5a18858a4c82e910f1462622b)
