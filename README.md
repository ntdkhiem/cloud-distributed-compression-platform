# Distributed Compression as a Service

A scalable, event-driven file compression system that processes large datasets in parallel using Go and Google Cloud services.

> [!NOTE]
> This is under active development. Follow my progress by watching on YouTube!
> 
> [CODING JOURNEY HERE](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL)

## Architecture Diagram

<img width="524" height="388" alt="DCaS V1 drawio" src="https://github.com/user-attachments/assets/2ed04763-95ec-4def-87bc-558793985f47" />

To be evolved soon.

## Architecture Components
- **Manager Service:** Handles file uploads and creates jobs.
- **Worker Service:** Processes jobs using Huffman's algorithm, lossless data compression.
- **Status Service (Planned):** Monitors updates on certain jobs in real-time.
- **Message Queue (Pub/Sub):** Stores queue of asynchronous jobs.
- **Object Storage (Google Cloud Storage):** Stores files.
- **Status Database (Planned):** (like Google Firebase) stores status for real-time updates.

## Quick Start
_(Setup and run instructions are not yet available for this project)_

### Prerequisites
- Go
- Docker
- Kubernetes
- Google Cloud Platform account (enable Pub/Sub, GCS, GKE, Firebase APIs)

### Setup
_(Instructions to be added)_

## Components
### Manager Service
- Accepts file uploads.
- Streams files to storage while simultaneously calculate character frequency table.
- Distributes compression/decompression jobs to message queue.
- [TODO] Updates job status in Status DB.

### Worker Service
- Subscribes to compression/decompression jobs.
- Downloads original/compressed file and character frequency table from storage.
- Builds Huffman tree.
- Encodes/Decodes file and then uploads to storage.
- [TODO] Updates job status in Status DB.

### Status Service
- Queries from Status DB and returns updates.

### Message Queue (Pub/Sub)
- Enforces message schemas.
- Enforces pull-based subscription model only.
- Redelivers unacknowledged messages every x seconds for x times.
- No Dead Letter Queue at the moment.

### Object Storage (Cloud Storage)
- Stores original file.
- Stores character frequency table.
- Stores metadata file: `filename`, `og_size`, `cp_size`.
- Stores compressed file.

### Status Database (Firebase)
- Provides highly available, low-latency NoSQL data.
- Stores statuses for real-time updates.

## Known Risks/Limitations
- This design assumes a "happy path". In a distributed system, any network call can fail and any message can be delivered more than once.
- A flood of traffic to Manager Service can lead to massive scalability issues, especially with large files, in which the service is designed to stream one file at a time.
- How the heck can I even debug this asynchronous architecture?
- Google managed services are not 100% SLA.
- Services can access GCS and Pub/Sub with no known rate limits, quotas.

## Todo
- Write unit tests and integration tests for manager and worker services.
- Containerize services with Docker.
- Use Kubernetes (GKE) to manage, scale, and orchestrate the services.
- Build Continuous Integration (CI) pipeline with Github Actions.
- Build Continuous Deployment (CD) pipeline with ArgoCD.
- Integrate Google Cloud Operations Suite for monitoring, logging, and tracing.
- Implement retries logic at streaming data, sending/receiving messages to/from Message Queue, etc.
- Create a budget plan to estimate the cost of running this system on Google Cloud Platform (GCP).
- Split file (>= 50GB) in chunks for parallel compression. **Requires architecture redesign**

## Development
The project is currently in active development. You can follow the progress by watching [my YouTube playlist](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL).

_Inspired by Silicon Valley series and [codingchallenges.fyi](https://codingchallenges.fyi/challenges/challenge-huffman) :)_
