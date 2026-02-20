# LibP2P Resource Manager Limits

This document explains how the libp2p resource manager parameters are calculated in `calculateOptimalLimits()` (`libp2putils/libp2p_client.go`).

## Network Topology

- **Leader node**: Connected to up to `maxOperators` (default 32) regular nodes
- **Regular node**: Connected to 1 leader node

## Broadcast Optimization

When broadcasting CVS, COS, or secret values, the leader is configured to broadcast to **all** operators (i.e., `maxOperators` recipients per value). `calculateOptimalLimits()` in `libp2putils/libp2p_client.go`.

## Stream Protocols

| Direction (relative to leader) | Protocol | Description |
|---|---|---|
| Inbound | `/register` | Regular node registration |
| Inbound | `/cvs` | CVS commitment submission |
| Inbound | `/cos` | COS submission |
| Inbound | `/secretValue` | Secret value revelation |
| Inbound | `/acknowledgment` | Broadcast receipt acknowledgment |
| Outbound | `/cvsBroadcast` | Broadcast CVS to other operators (skip originator) |
| Outbound | `/cosBroadcast` | Broadcast COS to other operators (skip originator) |
| Outbound | `/secretBroadcast` | Broadcast secret to other operators (skip originator) |
| Outbound | `/sendSecretValue` | Request secret reveal from specific operator |

## Memory Constants Explained

The memory calculation uses specific constants based on libp2p's resource consumption patterns:

| Constant | Value | Explanation |
|---|---|---|
| `512 * 1024` | 512 KB | Memory per connection. Each TCP/QUIC connection requires buffers for send/receive, connection state, encryption state (TLS/Noise), and multiplexer state. 512KB is a conservative estimate. |
| `32 * 1024` | 32 KB | Memory per stream. Each multiplexed stream needs read/write buffers and protocol state. Streams are lightweight compared to connections. |
| `256 * 1024 * 1024` | 256 MB | Base memory overhead for the leader node. Includes libp2p host, DHT (if used), peerstore, connection manager, and application-level data structures. |
| `32 * 1024 * 1024` | 32 MB | Base memory overhead for regular nodes. Smaller since regular nodes have fewer responsibilities and maintain less state. |
| `× 4` (FD multiplier) | 4 FDs/conn | Each connection may use multiple file descriptors: the socket itself, potential upgrade connections, and internal pipes for the multiplexer. |

### Memory Formula

**Leader Node:**
```
Memory = (connections × 512KB) + (streamsInbound × 32KB) + (streamsOutbound × 32KB) + 256MB base
```

**Regular Node:**
```
Memory = ((streamsInbound + streamsOutbound) × 32KB) + 32MB base
```

## Leader Node Calculations

Rounds execute sequentially. Values below are for a single round with `maxOperators = 32`.

### Connections

| Parameter | Base Value | With 25% Buffer | Formula |
|---|---|---|---|
| `ConnsInbound` | 32 | 40 | `maxOperators + 25%` |
| `ConnsOutbound` | 32 | 40 | Same as inbound (symmetric) |

### Streams (per round)

**Inbound = 3200 streams (base):**

| Source | Count | Calculation |
|---|---|---|
| Registration | 32 | 1 per operator |
| CVS commits | 32 | 1 per operator |
| CVS acks | 1024 | 32 CVS values × 32 operators acknowledging each |
| COS submissions | 32 | 1 per operator |
| COS acks | 1024 | 32 COS values × 32 operators acknowledging each |
| Secret submissions | 32 | 1 per operator |
| Secret acks | 1024 | 32 secret values × 32 operators acknowledging each |

**Outbound = 3072 streams (base):**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 1024 | 32 CVS values × 32 operators |
| COS broadcasts | 1024 | 32 COS values × 32 operators |
| Secret broadcasts | 1024 | 32 secret values × 32 operators |

A **25% safety margin** is applied, giving final values of **4000 inbound** and **3840 outbound**.

### Per-Connection Limits

| Parameter | Base Value | With 25% Buffer | Reasoning |
|---|---|---|---|
| `ConnBaseLimit.StreamsInbound` | 100 | 125 | 32 CVS acks + 32 COS acks + 32 Secret acks + 4 submissions |
| `ConnBaseLimit.StreamsOutbound` | 289 | 361 | `3 × 32 × 3 retries + 1` = 289 |

The outbound per-connection limit accounts for:
- 32 CVS broadcasts
- 32 COS broadcasts
- 32 Secret broadcasts
- 1 secret value request
- × 3 retries for reliability

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 160 | `estimatedConnections × 4` (4 FDs per connection for socket + multiplexer) |
| `Memory` | ~521MB | `(40 × 512KB) + (4000 × 32KB) + (3840 × 32KB) + 256MB` |

## Regular Node Calculations

A regular node only connects to the leader. It receives broadcasts for **all** operators (including its own) and sends acks.

### Connections

| Parameter | Value | Reasoning |
|---|---|---|
| `ConnsInbound` | 2 | Leader + potential reconnection |
| `ConnsOutbound` | 2 | Leader + backup |

### Streams (per round, worst case)

The leader retries broadcasts up to **3 times** if no acknowledgment is received.

**Inbound = 288 streams (base), 360 with 25% buffer:**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 96 | 32 operators × 3 retries |
| COS broadcasts | 96 | 32 operators × 3 retries |
| Secret broadcasts | 96 | 32 operators × 3 retries |

**Outbound = 99 streams (base), 123 with 25% buffer:**

| Source | Count | Calculation |
|---|---|---|
| CVS commit | 1 | Own commitment |
| COS submission | 1 | Own COS |
| Secret submission | 1 | Own secret |
| ACKs | 96 | 1 ack per received broadcast (32 operators × 3 phases) |

### Per-Connection Limits

Per-connection limits equal system limits since the regular node has only one meaningful connection (to the leader).

| Parameter | Base Value | With 25% Buffer |
|---|---|---|
| `StreamsInbound` | 288 | 360 |
| `StreamsOutbound` | 99 | 123 |

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 32 | Minimal — only 2 connections × 4 FDs + buffer |
| `Memory` | ~47MB | `((360 + 123) × 32KB) + 32MB base` |

## Summary

### Final Values Summary

| Node Type | Parameter | Final Value |
|---|---|---|
| Leader | System Streams Inbound | 4000 |
| Leader | System Streams Outbound | 3840 |
| Leader | Conn Streams Inbound | 125 |
| Leader | Conn Streams Outbound | 361 |
| Leader | Memory | ~521MB |
| Regular | System Streams Inbound | 360 |
| Regular | System Streams Outbound | 123 |
| Regular | Memory | ~47MB |
