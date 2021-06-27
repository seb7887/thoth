# Thoth

## Overview

**Thoth** is a Golang MQTT Broker compatible with eclipse paho client and mosquitto-client. It achieves high throughput and concurrency with less use of CPU and memory.

## Getting Started

First, you need to provide the environment variables by copying the `env.sample` to an `.env` one and filling up with the corresponding values

```bash
cp env.sample .env
```

A Makefile is provided to facilitate actions like format, building the executable and running it.

You just can run it by executing

```bash
make run
```

## Client Authentication

TODO

## Reference

- Surgemq (https://github.com/surgemq/surgemq)

## TODO

- docs
- gRPC client connection pool
