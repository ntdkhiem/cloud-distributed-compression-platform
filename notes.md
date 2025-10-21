# NOTES FOR SELF (AFTER)
====
chatgpt's approach is bad because the idea of building Huffman prefix tree won't work if the file
is splitted. See why below.

possible solutions:
solution 1: if I follow chatgpt's approach, I can split the file into a predefined number of chunks 
and output the compressed chunks into respective files. The decompress function will read all chunks
and merge them back together.
+ benefits: potentially reduces the time
* drawbacks: wayyyy overhead with chunks coordination. Can this even be considered as a solution for
distributed system?


solution 2: load up the entire content to build the tree then split the body into chunks to compress.
+ benefits: not having to worry about multiple chunks and the overhead of coordinating them.
* drawbacks: defeating the purpose of splitting content because we're loading in the entire content
anyway. 

Verdict: let's go with the second solution because it seems to be easier to implement since we 
already have the foundation.

How to run: 
- create GOOGLE_APPLICATION_CREDENTIALS env 
https://cloud.google.com/docs/authentication/application-default-credentials#GAC
- create GCS_BUCKET env

# COMPRESSED STRUCTURE 
====

## Before "distributed" compressing
a (uint16: 2 bytes) -> length of header.
b (uint32: 4 bytes) -> symbol in binary form.
c (string: 4 bytes) -> Huffman assigned code.
d (byte: 1 byte) -> number of bits needed to reach the symbol from the tree.
e (byte: 1 byte) -> number of padded zeros after encoding body.
f (byte: 1 byte) -> a segment of the body.
structure: a + [(b + c + d)...] + e + [f...]
EX: 500MB file takes ~45s to compress.

## After "distributed" compressing
a (uint16: 2 bytes) -> length of header.
b (uint32: 4 bytes) -> symbol in binary form.
c (string: 4 bytes) -> Huffman assigned code.
d (byte: 1 byte) -> number of bits needed to reach the symbol from the tree.
e (byte: 1 byte) -> number of padded zeros after encoding body.
f (byte: 1 byte) -> a segment of the body.
structure: a + [(b + c + d)...] + [(e + f)...]
EX: TBA

tradeoff: sum([(e + f)...]) >> sum(e + [f...]) for "maybe" faster time complexity

RESULT:
- with splitting into chunks, the efficiency is only ~30% faster. I will go ahead with this 
implementation anyway because why not...
- (SOLVED) the challenge right now is how do I decompress this... (NICEEEEE!)
- (SOLVED) use goroutines on decompression

# Next: create HTTP client and server
======
## FIRST REVISION
---
### client
takes in text file, split into CHUNKS if size >= 500MB. Passes to the server one by one.
- get frequency table from the file to be sent to the server. Once confirmed from server, start 
splitting into chunks to send over the server.  

- define a route /upload that accepts POST method
- when a user submits a text file in this route, it will validate the file's extension.
- request for a unique ID from the server.
- build a frequency table of unique characters
- send the table to the server (127.0.0.1:8080/table?id=[ID])
- if success, divide the text file into 3 chunks if the file size is greater than 100MB.
- send each chunk to the server sequentially (FUTURE: in parallel)  

- session affinity? memory db? 
- latency? throughput?

### server
takes in each chunk or entire chunk, create frequency table, only starts building the tree once has received all. 
- returns status 200.
- include a temp link to track progress?

+ how do I know when the compression/decompression work is running/finished?
+ how do I ensure the chunks being sent are associated to a particular file?
+ how can the server respond back to the caller? no need?
+ if the server is a managed instance of stateless VMs, or cluster of stateless containers, how do I 
ensure the chunks are distributed evenly and still able to merge into one whole?
+ (future) after a request failed to process, how do I ensure the lost chunks can still be sent after?
+ (far far future) CAP theorem.

## SECOND REVISION
---
### manager
- takes in text file, build character frequency table, assign incoming request with UID.
- send the table along with UID to the worker for Huffman tree building
* this will be another service called Huffman Tree Coordinator or just coordinator.
    - more than one worker can reference their tree with assigned UID to this service.

### worker
- mainly building header, body, and send back the result

### Flaws
- the issue with manager holding the entire file content in the memory while building character
frequency table is the potential significant amount of memory that can cause memory overflow, DoS,
- an enterprise-scale application should be able to handle gigabytes of memory in the most efficient
way.
- right now, the communication line between manager and worker is synchronous so there are some blocking
calls somewhere during the interaction. I should find a way to delegate these asynchronously.
- if worker is the one storing chunks of data while building the huffman tree and compress the data,
what if the worker fails? it will lose everything and wouldn't be possible to recover the work.
- this stateful tracking of workers and managers can make horizontal scaling very complex and inefficient.
- i think I should start optimize for scalability now before wasting more time developing this "two-form" 
monolithic.
- FUTURE WORK: implement retries.

## THIRD REVISION
---
what if instead of having the manager handling data content in memory, I can delegate that to a 
robust data storage like Google Cloud Storage? 
what if instead of having the synchronous communication line, I can use a dedicated communication 
orchestrator, a message queue like Redis, use Pub/Sub.

### manager
- receives the file upload from the client.
- streams content directly to GCS while simultaneously calculating the character freq. table on-the-fly.
Keep the memory footprint small.
- once the upload is complete and the table is built, manager should upload to GCS under the same job ID 
so the worker can retrieve later.
- sends a message to MQ with job's id, object's path, table's path.
    - publishes a message to Pub/Sub with predefined schema
- immediately returns 202 code with job's id.

### worker 
- the worker should be the consumer of the MQ, listening for new job message.
    - follow Pub/Sub created subscription model.
- once receives the message, parses the content.
- downloads frequency table to build the huffman tree
- download/stream? the original file from GCS, in chunks (1MB?)
- for each chunk, compress using Huffman and uploads back to GCS
(can I do this if the chunks split the bits of a character?)
- after processing all chunks, compress all parts into one.

BENEFITS: scalability, jobs won't get lost if workers die, low memory footprint
DRAWBACKS: infrastructure cost, more complexity, what happens if multiple workers receive same 
message at the same time?

#### Work
- streamed file content to GCS
- streamed character frequency table to GCS
- created topic
- created pull subscription model
- added publisher, subscriber roles to service account
- published a message to topic
- worker pulled message and built huffman coding tree
- why am I using streaming?


worker: downloads file content down and compresses it while simultaneously writes it 
to another Reader.
- will this overload the worker if the file is more than say 100MB?
- what's the latency? throughput?
- alternative? stream in trunks? how?

----

early optimization is killing my productivity. I'll revert back to original version now.



















