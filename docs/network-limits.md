# LibP2P Resource Manager Limits

This document explains how the libp2p resource manager parameters are calculated in `calculateOptimalLimits()` (`libp2putils/libp2p_client.go`).

## Network Topology

- **Leader node**: Connected to up to `maxOperators` (default 32) regular nodes
- **Regular node**: Connected to 1 leader node

## Stream Protocols

| Direction (relative to leader) | Protocol | Description |
|---|---|---|
| Inbound | `/register` | Regular node registration |
| Inbound | `/cvs` | CVS commitment submission |
| Inbound | `/cos` | COS submission |
| Inbound | `/secretValue` | Secret value revelation |
| Inbound | `/acknowledgment` | Broadcast receipt acknowledgment |
| Outbound | `/cvsBroadcast` | Broadcast CVS to all operators |
| Outbound | `/cosBroadcast` | Broadcast COS to all operators |
| Outbound | `/secretBroadcast` | Broadcast secret to all operators |

## Leader Node Calculations

Rounds execute sequentially. Values below are for a single round with `maxOperators = 32`.

### Connections

| Parameter | Value | Formula |
|---|---|---|
| `ConnsInbound` | 40 | `maxOperators + 25% buffer` |
| `ConnsOutbound` | 40 | Same as inbound (symmetric) |

### Streams (per round)

**Inbound = 3200 streams:**

| Source | Count | Calculation |
|---|---|---|
| Registration | 32 | 1 per operator |
| CVS commits | 32 | 1 per operator |
| CVS acks | 1024 | 32 CVS values × 32 operators acknowledging each |
| COS submissions | 32 | 1 per operator |
| COS acks | 1024 | 32 COS values × 32 operators acknowledging each |
| Secret submissions | 32 | 1 per operator |
| Secret acks | 1024 | 32 secret values × 32 operators acknowledging each |

**Outbound = 3072 streams:**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 1024 | 32 CVS values × 32 operators |
| COS broadcasts | 1024 | 32 COS values × 32 operators |
| Secret broadcasts | 1024 | 32 secret values × 32 operators |

A **25% safety margin** is applied to both, giving final values of **4000 inbound** and **3840 outbound**.

### Per-Connection Limits

| Parameter | Value | Reasoning |
|---|---|---|
| `ConnBaseLimit.StreamsInbound` | 96 | Per regular node: 32 CVS acks + 32 COS acks + 32 Secret acks |
| `ConnBaseLimit.StreamsOutbound` | 10 | Per regular node: broadcasts + overhead |

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 160 | `estimatedConnections × 4` (~4 FDs per connection) |
| `Memory` | ~530MB | `connections × 512KB + streams × 32KB + 256MB base` |

## Regular Node Calculations

A regular node only connects to the leader. It receives broadcasts for all operators and sends acks.

### Connections

| Parameter | Value | Reasoning |
|---|---|---|
| `ConnsInbound` | 2 | Leader + potential reconnection |
| `ConnsOutbound` | 2 | Leader + backup |

### Streams (per round, worst case)

The leader retries broadcasts up to **3 times** if no acknowledgment is received.

**Inbound = `3 × maxOperators × 3 retries` = 288 streams:**

| Source | Count | Calculation |
|---|---|---|
| CVS broadcasts | 96 | 32 operators × 3 retries |
| COS broadcasts | 96 | 32 operators × 3 retries |
| Secret broadcasts | 96 | 32 operators × 3 retries |

**Outbound = `3 + (3 × maxOperators)` = 99 streams:**

| Source | Count | Calculation |
|---|---|---|
| CVS commit | 1 | Own commitment |
| COS submission | 1 | Own COS |
| Secret submission | 1 | Own secret |
| ACKs | 96 | 1 ack per successfully received broadcast (32 operators × 3 phases) |

### Per-Connection Limits

Per-connection limits equal system limits since the regular node has only one meaningful connection (to the leader).

### File Descriptors and Memory

| Parameter | Value | Formula |
|---|---|---|
| `FD` | 32 | Minimal — only 2 connections |
| `Memory` | ~44MB | `(inbound + outbound streams) × 32KB + 32MB base` |

