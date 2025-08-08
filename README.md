# Cloud Distributed Compression Platform
_As a complete noob in Cloud, this is a challenge for myself to learn everything by building an enterprise-scale cool thing from the ground up using Go._

A cloud‑native distributed compression platform that splits large files into chunks, compresses them in parallel across a cluster of worker nodes, and seamlessly merges the results. 

_Inspired by Silicon Valley series and [codingchallenges.fyi](https://codingchallenges.fyi/challenges/challenge-huffman) :)_

[Youtube Code-Along](https://www.youtube.com/playlist?list=PLSg4pGV1EkBo1JCfXl4zZoHkbFe4zk_EL)

# TODO
- [X] Develop Huffman's algorithm to be used for lossless data compression.
- [X] Create a pipeline that takes in data for encoding or decoding with the algorithm.
- [X] Create simple tests for the algorithm and the pipeline.
- [ ] Split input file into chunks for distributed compressing for files > 100MB.
- [ ] Turn the pipeline into a REST API that accepts files (or streams) and returns compressed results.
- [ ] Containerize the service and add proper environment configs.
- [ ] Orchestrate the compression service on a cluster using Kubernetes.
- [ ] Expose metrics with Prometheus and see them in Grafana.
- [ ] Implement logging & tracing with OpenTelemetry.
- [ ] Split large files into chunks and process them in parallel across pods using pub/sub pattern.
- [ ] Deploy the system to Google Cloud Platform.
- [ ] a) Use Google Kubernetes Engine
- [ ] b) Store input/output in Google Cloud Storage
- [ ] c) Use Pub/Sub for chunk distribution
- [ ] d) Add CI/CD with Cloud Build
- [ ] e) Secure the infrastructure by implementing proper IAM policies
- [ ] (maybe) Support multiple algorithms other than Huffman 
- [ ] (maybe) Look into arithmetic coding (adaptive version) -- known to be better at distributed compression
