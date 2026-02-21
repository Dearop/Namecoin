# Namecoin-on-Peerster

A decentralized naming and resolution system inspired by **Namecoin**, built on top of a peer-to-peer network architecture developed in EPFL's *Decentralized Systems Engineering* course.

This project extends a distributed P2P overlay with blockchain-based name registration, ownership, and resolution mechanisms, enabling trustless identity and naming without centralized authorities.

---

## Overview

Namecoin-on-Peerster implements a decentralized naming service where peers collectively maintain a blockchain-backed registry mapping human-readable names to values.

Key features include:

- Peer-to-peer networking and message propagation
- Proof-of-Work blockchain consensus
- Decentralized name registration and ownership
- Distributed state synchronization across peers
- Fork handling and chain selection
- Integrated wallet and mining support
- Web-based GUI for network interaction

The system demonstrates core challenges in decentralized infrastructure, including consensus, state consistency, and adversarial environments.

---

## Architecture

Each node operates as an autonomous peer responsible for:

- maintaining local blockchain state
- validating and propagating blocks
- resolving naming records
- participating in mining and consensus

Peers communicate through a distributed overlay network and synchronize naming data through blockchain replication.

---

## Running a Node

### Requirements

- Go ≥ 1.23

### Start a peer

```sh
cd gui
go run gui.go start