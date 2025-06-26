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



## API

- **Admin API**
  - `GET /health` - Get health check
  - `GET /api/v1/status` - Get server status


- **Build API**
  - `POST /api/v1/builds` - Create new build
  - `GET /api/v1/builds/stats` - Get build statistics
  - `GET /api/v1/builds/order` - Get topological build order
  - `GET /api/v1/builds/{id}` - Get specific build


- **Rule API**
  - `POST /api/v1/rules` - Create new rule
  - `GET /api/v1/rules/{name}/targets` - Get targets using a rule
  - `GET /api/v1/rules/{name}` - Get specific rule


- **Target API**
  - `GET /api/v1/targets` - Get all targets
  - `GET /api/v1/targets/{path}/dependencies` - Get target dependencies
  - `GET /api/v1/targets/{path}/reverse_dependencies` - Get target reverse dependencies
  - `PUT /api/v1/targets/{path}/status` - Update target status
  - `GET /api/v1/targets/{path}` - Get specific target


- **Analysis API**
  - `GET /api/v1/analysis/cycles` - Find circular dependencies


- **Debug API**
  - `GET /api/v1/debug/quads` - Debug quad information


- **Load API**
  - `POST /api/v1/load` - Load ninja file



## Proto

```proto
syntax = "proto3";

package distninja;

option go_package = "github.com/distninja/distninja/server/proto;proto";

service DistNinjaService {
  // Admin
  rpc Health(HealthRequest) returns (HealthResponse);
  rpc Status(StatusRequest) returns (StatusResponse);

  // Build
  rpc CreateBuild(CreateBuildRequest) returns (CreateBuildResponse);
  rpc GetBuild(GetBuildRequest) returns (NinjaBuild);
  rpc GetBuildStats(BuildStatsRequest) returns (BuildStatsResponse);
  rpc GetBuildOrder(BuildOrderRequest) returns (BuildOrderResponse);

  // Rule
  rpc CreateRule(CreateRuleRequest) returns (CreateRuleResponse);
  rpc GetRule(GetRuleRequest) returns (NinjaRule);
  rpc GetTargetsByRule(GetTargetsByRuleRequest) returns (GetTargetsByRuleResponse);

  // Target
  rpc GetAllTargets(GetAllTargetsRequest) returns (GetAllTargetsResponse);
  rpc GetTarget(GetTargetRequest) returns (NinjaTarget);
  rpc GetTargetDependencies(GetTargetDependenciesRequest) returns (GetTargetDependenciesResponse);
  rpc GetTargetReverseDependencies(GetTargetReverseDependenciesRequest) returns (GetTargetReverseDependenciesResponse);
  rpc UpdateTargetStatus(UpdateTargetStatusRequest) returns (UpdateTargetStatusResponse);

  // Analysis
  rpc FindCycles(FindCyclesRequest) returns (FindCyclesResponse);

  // Debug
  rpc DebugQuads(DebugQuadsRequest) returns (DebugQuadsResponse);

  // Load
  rpc LoadNinjaFile(LoadNinjaFileRequest) returns (LoadNinjaFileResponse);
}

// Admin
message HealthRequest {}
message HealthResponse {
  string status = 1;
  string timestamp = 2;
}

message StatusRequest {}
message StatusResponse {
  string service = 1;
  string uptime = 2;
}

// Build
message CreateBuildRequest {
  string build_id = 1;
  string rule = 2;
  map<string, string> variables = 3;
  string pool = 4;
  repeated string inputs = 5;
  repeated string outputs = 6;
  repeated string implicit_deps = 7;
  repeated string order_deps = 8;
}
message CreateBuildResponse {
  string status = 1;
  string build_id = 2;
}

message GetBuildRequest { string id = 1; }

message BuildStatsRequest {}
message BuildStatsResponse {
  map<string, int64> stats = 1;
}

message BuildOrderRequest {}
message BuildOrderResponse {
  repeated string build_order = 1;
}

// Rule
message CreateRuleRequest {
  string name = 1;
  string command = 2;
  string description = 3;
  map<string, string> variables = 4;
}
message CreateRuleResponse {
  string status = 1;
  string name = 2;
}

message GetRuleRequest { string name = 1; }
message GetTargetsByRuleRequest { string rule_name = 1; }
message GetTargetsByRuleResponse { repeated NinjaTarget targets = 1; }

// Target
message GetAllTargetsRequest {}
message GetAllTargetsResponse { repeated NinjaTarget targets = 1; }

message GetTargetRequest { string path = 1; }

message GetTargetDependenciesRequest { string path = 1; }
message GetTargetDependenciesResponse { repeated NinjaFile dependencies = 1; }

message GetTargetReverseDependenciesRequest { string path = 1; }
message GetTargetReverseDependenciesResponse { repeated NinjaTarget reverse_dependencies = 1; }

message UpdateTargetStatusRequest {
  string path = 1;
  string status = 2;
}
message UpdateTargetStatusResponse { string status = 1; }

// Analysis
message FindCyclesRequest {}
message FindCyclesResponse {
  repeated Cycle cycles = 1;
  int32 cycle_count = 2;
}
message Cycle { repeated string nodes = 1; }

// Debug
message DebugQuadsRequest {
  int32 limit = 1;
}
message DebugQuadsResponse {
  string message = 1;
  int32 limit = 2;
}

// Load
message LoadNinjaFileRequest {
  string file_path = 1;
  string content = 2;
}
message LoadNinjaFileResponse {
  string status = 1;
  string message = 2;
  map<string, int64> stats = 3;
  string build_time = 4;
}

// Ninja
message NinjaBuild {
  string id = 1;
  string type = 2;
  string build_id = 3;
  string rule = 4;
  string variables = 5;
  string pool = 6;
}

message NinjaFile {
  string id = 1;
  string type = 2;
  string path = 3;
  string file_type = 4;
}

message NinjaRule {
  string id = 1;
  string type = 2;
  string name = 3;
  string command = 4;
  string description = 5;
  string variables = 6;
}

message NinjaTarget {
  string id = 1;
  string type = 2;
  string path = 3;
  string status = 4;
  string hash = 5;
  string build = 6;
}
```



## License

Project License can be found [here](LICENSE).



## Reference

- [cayley](https://github.com/distninja/cayley)
- [ninja](https://github.com/ninja-build/ninja)
- [ninja-build](https://gist.github.com/craftslab/a9cacfa5a18858a4c82e910f1462622b)
