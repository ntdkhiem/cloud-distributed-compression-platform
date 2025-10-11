package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strconv"
)

const CHUNKS_COUNT = 3

type lookupItem struct {
	char rune
	freq uint64
	code string
	bits int
}

type node struct {
	value *lookupItem
	left  *node
	right *node
}

type prefixTable map[rune]*lookupItem
type priorityQueue []*node

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].value.freq > pq[j].value.freq
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x any) {
	item := x.(*node)
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*pq = old[0 : n-1]
	return item
}

func buildTree(root *node, code string, bits int) {
	if root == nil {
		return
	}
	item := root.value
	// if node is a leaf, assign code and bits
	if item.char != 0 {
		item.code = code
		item.bits = bits
		return
	}
	buildTree(root.left, code+"0", bits+1)
	buildTree(root.right, code+"1", bits+1)
}

// TODO: assuming everything works like a champ. Add error handlers like
// a normal person somewhere here
func buildHuffmanTree(freqTable map[rune]uint64) (priorityQueue, prefixTable, error) {
	pq := make(priorityQueue, len(freqTable))
	pt := make(prefixTable)

	i := 0
	for k, v := range freqTable {
		val := &lookupItem{
			char: k,
			freq: -v,
		}
		pt[k] = val
		pq[i] = &node{
			value: pt[k],
		}
		i++
	}
	heap.Init(&pq)
	log.Printf("INFO: built prefix table")

	for pq.Len() > 1 {
		n1 := heap.Pop(&pq).(*node)
		n2 := heap.Pop(&pq).(*node)
		newnode := node{}
		newnode.value = &lookupItem{
			freq: n1.value.freq + n2.value.freq,
		}
		if n1.value.freq > n2.value.freq {
			newnode.right = n1
			newnode.left = n2
		} else {
			newnode.right = n2
			newnode.left = n1
		}
		heap.Push(&pq, &newnode)
	}
	log.Printf("INFO: built priority queue")
	buildTree(pq[0], "", 0)
	log.Printf("INFO: built Huffman Tree")
	return pq, pt, nil
}

func buildHeader(root *node, w *bytes.Buffer) error {
	if root == nil {
		return nil
	}
	item := root.value
	if item.char != 0 {
		data := make([]byte, 9)
		// first four bytes: character code
		binary.LittleEndian.PutUint32(data[:4], uint32(item.char))
		// next four bytes: Huffman assigned code
		// WARNING: Huffman codes can be longer than 32 bits in some implementations, but for
		// simplicity, I force it to be <= 32 bits.
		v, err := strconv.ParseUint(item.code, 2, 32)
		if err != nil {
			return err
		}
		smallV := uint32(v)
		binary.LittleEndian.PutUint32(data[4:8], smallV)
		// last byte: bits
		data[8] = byte(item.bits)
		// write to buffer
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("Failed to write bytes to header: %w", err)
		}
	}
	if err := buildHeader(root.left, w); err != nil {
		return err
	}
	if err := buildHeader(root.right, w); err != nil {
		return err
	}
	return nil
}

func compress(root *node, pt prefixTable, bodyData *bufio.Reader, w *io.PipeWriter) {
	var headerBuffer bytes.Buffer
	err := buildHeader(root, &headerBuffer)
	if err != nil {
		log.Printf("ERROR: failed to build header: %v", err)
		return
	}
	headerLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(headerLen, uint16(headerBuffer.Len()))
	_, err = w.Write(headerLen)
	if err != nil {
		log.Printf("ERROR: failed to write length header: %v", err)
	}
	_, err = headerBuffer.WriteTo(w)
	if err != nil {
		log.Printf("ERROR: failed to write header content: %v", err)
		return
	}
	// log.Printf("Write %d bytes of header.", n)

	// write body
	var currentByte byte
	var bitCount int
	for {
		char, _, err := bodyData.ReadRune()
		if err != nil {
			// If we've reached the end of the file, we're done.
			if err == io.EOF {
				break
			}
			// Otherwise, it's a real error.
			log.Printf("ERROR: failed to read file to build freq. table: %v", err)
			return
		}
		item := pt[char]
		for _, bit := range item.code {
			currentByte <<= 1 // shift left to make space for next bit
			if bit == '1' {
				currentByte |= 1
			}
			bitCount++
			if bitCount == 8 {
				w.Write([]byte{currentByte}) // TODO: find out the drawbacks of this.
				currentByte = 0
				bitCount = 0
			}
		}
	}
	// flush the remaining bits (pad with 0s on the right)
	// TODO: Do I need this? Does it auto pad for me?
	var paddedZeros uint8 = 0
	if bitCount > 0 {
		paddedZeros = uint8(8 - bitCount)
		currentByte <<= paddedZeros  // shift remaining bits to fill the byte
		w.Write([]byte{currentByte}) // TODO: find out the drawbacks of this.
	}
}
