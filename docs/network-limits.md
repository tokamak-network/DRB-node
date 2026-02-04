# LibP2P Resource Manager Limits

This document explains how the libp2p resource manager parameters are calculated in `calculateOptimalLimits()` (`libp2putils/libp2p_client.go`).

## Network Topology

- **Leader node**: Connected to up to `maxOperators` (default 32) regular nodes
- **Regular node**: Connected to 1 leader node

## Broadcast Optimization

When broadcasting CVS, COS, or secret values, the leader **skips the originating operator**. The operator who submitted the data already has it locally, so there's no need to send it back to them. This reduces network overhead and stream usage.

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

**Inbound = 3104 streams (base):**

| Source | Count | Calculation |
|---|---|---|
| Registration | 32 | 1 per operator |
| CVS commits | 32 | 1 per operator |
| CVS acks | 992 | 32 CVS values × 31 operators acknowledging each (originator skipped) |
| COS submissions | 32 | 1 per operator |
| COS acks | 992 | 32 COS values × 31 operators acknowledging each |
| Secret submissions | 32 | 1 per operator |
| Secret acks | 992 | 32 secret values × 31 operators acknowledging each |

**Outbound = 2976 streams (base):**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 992 | 32 CVS values × 31 operators (originator skipped) |
| COS broadcasts | 992 | 32 COS values × 31 operators (originator skipped) |
| Secret broadcasts | 992 | 32 secret values × 31 operators (originator skipped) |

A **25% safety margin** is applied, giving final values of **3880 inbound** and **3720 outbound**.

### Per-Connection Limits

| Parameter | Base Value | With 25% Buffer | Reasoning |
|---|---|---|---|
| `ConnBaseLimit.StreamsInbound` | 97 | 121 | 31 CVS acks + 31 COS acks + 31 Secret acks + 4 submissions |
| `ConnBaseLimit.StreamsOutbound` | 280 | 350 | `3 × 31 × 3 retries + 1` = 280 |

The outbound per-connection limit accounts for:
- 31 CVS broadcasts (for other operators' CVS values)
- 31 COS broadcasts (for other operators' COS values)
- 31 Secret broadcasts (for other operators' secrets)
- 1 secret value request
- × 3 retries for reliability

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 160 | `estimatedConnections × 4` (4 FDs per connection for socket + multiplexer) |
| `Memory` | ~500MB | `(40 × 512KB) + (3880 × 32KB) + (3720 × 32KB) + 256MB` |

## Regular Node Calculations

A regular node only connects to the leader. It receives broadcasts for all **other** operators (not its own) and sends acks.

### Connections

| Parameter | Value | Reasoning |
|---|---|---|
| `ConnsInbound` | 2 | Leader + potential reconnection |
| `ConnsOutbound` | 2 | Leader + backup |

### Streams (per round, worst case)

The leader retries broadcasts up to **3 times** if no acknowledgment is received.

**Inbound = 279 streams (base), 348 with 25% buffer:**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 93 | 31 other operators × 3 retries |
| COS broadcasts | 93 | 31 other operators × 3 retries |
| Secret broadcasts | 93 | 31 other operators × 3 retries |

**Outbound = 96 streams (base), 120 with 25% buffer:**

| Source | Count | Calculation |
|---|---|---|
| CVS commit | 1 | Own commitment |
| COS submission | 1 | Own COS |
| Secret submission | 1 | Own secret |
| ACKs | 93 | 1 ack per received broadcast (31 other operators × 3 phases) |

### Per-Connection Limits

Per-connection limits equal system limits since the regular node has only one meaningful connection (to the leader).

| Parameter | Base Value | With 25% Buffer |
|---|---|---|
| `StreamsInbound` | 279 | 348 |
| `StreamsOutbound` | 96 | 120 |

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 32 | Minimal — only 2 connections × 4 FDs + buffer |
| `Memory` | ~47MB | `((348 + 120) × 32KB) + 32MB base` |

## Summary

### Broadcast Optimization Impact

The broadcast optimization (skipping originator) reduces:
- Leader outbound streams: 3072 → 2976 (before safety margin)
- Leader inbound acks: 3072 → 2976 (before safety margin)
- Regular node inbound broadcasts: 288 → 279
- Regular node outbound acks: 96 → 93

This optimization saves ~3% of network overhead while ensuring operators don't receive redundant copies of their own data.

### Final Values Summary

| Node Type | Parameter | Final Value |
|---|---|---|
| Leader | System Streams Inbound | 3880 |
| Leader | System Streams Outbound | 3720 |
| Leader | Conn Streams Inbound | 121 |
| Leader | Conn Streams Outbound | 350 |
| Leader | Memory | ~500MB |
| Regular | System Streams Inbound | 348 |
| Regular | System Streams Outbound | 120 |
| Regular | Memory | ~47MB |
