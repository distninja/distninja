# distninja

[![Build Status](https://github.com/distninja/distninja/workflows/ci/badge.svg?branch=main&event=push)](https://github.com/distninja/distninja/actions?query=workflow%3Aci)
[![Go Report Card](https://goreportcard.com/badge/github.com/distninja/distninja)](https://goreportcard.com/report/github.com/distninja/distninja)
[![License](https://img.shields.io/github/license/distninja/distninja.svg)](https://github.com/distninja/distninja/blob/main/LICENSE)
[![Tag](https://img.shields.io/github/tag/distninja/distninja.svg)](https://github.com/distninja/distninja/tags)



## Introduction

distninja is a distributed build system



## Features

- **Graph Database Power** - Uses [Cayley](https://github.com/cayleygraph/cayley)'s quad-based storage for complex relationships
- **Schema Support** - Structured data with Go struct mapping
- **Rich Queries** - Path-based queries for complex dependency analysis
- **Relationship Modeling** - Explicit modeling of all Ninja relationships
- **Cycle Detection** - Built-in circular dependency detection
- **Performance** - Efficient graph traversal and querying



## Deploy

### gRPC server

```
distninja --grpc-serve :9090
```

### HTTP server

```
distninja --http-serve :9091
```



## License

Project License can be found [here](LICENSE).



## Reference

- [ninja](https://github.com/ninja-build/ninja)
- [ninja-build](https://gist.github.com/craftslab/a9cacfa5a18858a4c82e910f1462622b)
