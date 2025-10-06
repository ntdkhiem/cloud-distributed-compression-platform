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
























