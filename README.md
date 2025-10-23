# Distributed Compression as a Service

A scalable, event-driven file compression system that processes large datasets in parallel using Go and Google Cloud services.

> [!NOTE]
> This is under active development. Follow my progress by watching on YouTube!
> 
> [CODING JOURNEY HERE](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL)

# Architecture Diagram

_(Architecture diagram to be created.)_

# Architecture Components
- **Manager Service:** Accepts file uploads, streams file to storage, and distributes compression/decompression jobs to workers.
- **Worker Service:** Subscribes to compression/decompression jobs, performs Huffman's algorithm, uploads to storage.
- **Status Service (Planned):** displays real-time updates on certain jobs to users.
- **Message Queue (Pub/Sub):** Manages the queue of asynchronous task orchestration for worker nodes in a pull-model subscription.
- **Object Storage (Google Cloud Storage):** Scalable object storage to hold the original, metadata, and final compressed files.
- **Document-based Database (Planned):** A real-time database (like Google Firebase) to be used by Status Service to receive updates from other services.

# Quick Start
_(Setup and run instructions are not yet available for this project)_

## Prerequisites
- Go
- Docker
- Kubernetes
- Google Cloud Platform account (enable Pub/Sub, GCS, GKE, Firebase APIs)

## Setup
_(Instructions to be added)_

# Development
The project is currently in active development. You can follow the progress by watching [my YouTube playlist](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL).

> [!TIP]
> To keep myself responsible, I document my journey developing this project from scratch so...  
> [FOLLOW MY CODING JOURNEY HERE](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL)
 
_As a complete noob in Cloud, this is a challenge for myself to learn everything by building an enterprise-scale cool thing from the ground up using Go._

A cloud‑native distributed compression platform that splits large files into chunks, compresses them in parallel across a cluster of worker nodes, and seamlessly merges the results. 

_Inspired by Silicon Valley series and [codingchallenges.fyi](https://codingchallenges.fyi/challenges/challenge-huffman) :)_
