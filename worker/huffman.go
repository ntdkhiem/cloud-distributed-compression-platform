package main

import (
	"bufio"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strconv"
)

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
func buildHuffmanTree(freqTable map[rune]uint64) (priorityQueue, error) {
	pq := make(priorityQueue, len(freqTable))
	pt := make(map[rune]*lookupItem)

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
	return pq, nil
}

func buildHeader(root *node, w *io.PipeWriter) error {
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
		// write to pipe
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

func compress(root *node, data *bufio.Reader, w *io.PipeWriter) {

	err := buildHeader(root, w)
	if err != nil {
		log.Printf("ERROR: failed to build header: %v", err)
		return
	}
	// return &header, err
	// header, err := buildHeader(pq[0])
	// if err != nil {
	// 	return nil, fmt.Errorf("Error building header: %w", err)
	// }
}
