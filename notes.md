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

## Before distributed compressing
a (uint16: 2 bytes) -> length of header.
b (uint32: 4 bytes) -> symbol in binary form.
c (string: 4 bytes) -> Huffman assigned code.
d (byte: 1 byte) -> number of bits needed to reach the symbol from the tree.
e (byte: 1 byte) -> number of padded zeros after encoding body.
f (byte: 1 byte) -> a segment of the body.
structure: a + [(b + c + d)...] + e + [f...]
EX: 500MB file takes ~45s to compress.

## After distributed compressing
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
- NEXT: use goroutines on decompression
