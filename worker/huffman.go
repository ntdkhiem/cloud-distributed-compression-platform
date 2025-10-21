package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
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

func (hn *node) addNode(char rune, code uint32, bits uint8) {
	if bits == 0 {
		hn.value = &lookupItem{
			char: char,
		}
		return
	}
	direction := code >> (bits - 1)
	if direction&1 == 0 {
		if hn.left == nil {
			hn.left = &node{}
		}
		hn.left.addNode(char, code, bits-1)
	} else {
		if hn.right == nil {
			hn.right = &node{}
		}
		hn.right.addNode(char, code, bits-1)
	}
}

func (hn *node) walker() func(int) (rune, bool) {
	t := hn
	return func(direction int) (rune, bool) {
		if direction == 0 {
			t = t.left
		} else {
			t = t.right
		}
		if t.value != nil {
			return t.value.char, true
		}
		return 0, false
	}
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
	buildTree(pq[0], "", 0)
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

func buildBody(pt prefixTable, bodyData *bufio.Reader) (*bytes.Buffer, uint8, error) {
	var output bytes.Buffer
	var curr byte
	var bitCount int

	c := 0

	for {
		c += 1
		char, _, err := bodyData.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, err
		}
		item := pt[char]

		for _, bit := range item.code {
			curr <<= 1 // shift left to make space for next bit
			if bit == '1' {
				curr |= 1
			}
			bitCount++
			if bitCount == 8 {
				output.WriteByte(curr)
				curr = 0
				bitCount = 0
			}
		}
	}
	// flush the remaining bits (pad with 0s on the right)
	var paddedZeros uint8 = 0

	if bitCount > 0 {
		paddedZeros = uint8(8 - bitCount)
		curr <<= paddedZeros // shift remaining bits to fill the byte
		output.WriteByte(curr)
	}
	return &output, paddedZeros, nil
}

func compress(root *node, pt prefixTable, bodyData *bufio.Reader) (*bytes.Buffer, error) {
	var fileBuf bytes.Buffer
	var headerBuf bytes.Buffer

	//--- Write header
	err := buildHeader(root, &headerBuf)
	if err != nil {
		return nil, fmt.Errorf("Failed to build header: %v", err)
	}
	headerLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(headerLen, uint16(headerBuf.Len()))
	_, err = fileBuf.Write(headerLen)
	if err != nil {
		return nil, fmt.Errorf("Failed to write length header: %v", err)
	}
	_, err = headerBuf.WriteTo(&fileBuf)
	if err != nil {
		return nil, fmt.Errorf("Failed to write header content: %v", err)
	}

	//--- Write body
	// TODO: implement chunks-based Huffman compression
	encodedBody, paddedZeros, err := buildBody(pt, bodyData)
	if err != nil {
		return nil, fmt.Errorf("Failed to encode body: %v", err)
	}
	err = fileBuf.WriteByte(paddedZeros)
	if err != nil {
		return nil, fmt.Errorf("Failed to write padded zeros: %v", err)
	}
	_, err = encodedBody.WriteTo(&fileBuf)
	if err != nil {
		return nil, fmt.Errorf("Failed to write encoded body: %w", err)
	}

	return &fileBuf, nil
}

// TODO: assuming this will go correctly, I need to have some good test cases
// for this.
func buildHuffmanTreeFromBin(headerBin []byte) *node {
	ht := node{}
	// character code + Huffman assigned code + bits -> 4 + 4 + 1 = 9 bytes
	for i := 0; i < len(headerBin); i += 9 {
		section := headerBin[i : i+9]
		char, _ := utf8.DecodeRune(section[0:4])
		code := binary.LittleEndian.Uint32(section[4:8])
		bits := section[8]
		ht.addNode(char, code, bits)
	}

	return &ht
}

func decompress(tree *node, bodyData *bufio.Reader, w *io.PipeWriter) error {
	walk := tree.walker()
	for {
		// extracting padded zeros section
		paddedZeros, err := bodyData.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("Error extracting padded 0s: %w", err)
		}
		// extracting body length section
		bodyBin := make([]byte, 4)
		_, err = bodyData.Read(bodyBin)
		if err != nil {
			return fmt.Errorf("Error extracting body length: %w", err)
		}
		bodyLen := binary.LittleEndian.Uint32(bodyBin)
		for i := range bodyLen {
			b, err := bodyData.ReadByte()
			if err != nil {
				return fmt.Errorf("Error decoding body: %w", err)
			}

			var endByte int
			if i+1 == bodyLen {
				// padded zeros situation
				endByte = int(paddedZeros)
			} else {
				endByte = 0
			}

			for i := 7; i >= endByte; i-- {
				bit := (b >> uint(i)) & 1
				v, ok := walk(int(bit))
				if ok {
					w.Write([]byte(string(v))) // is this the efficient way?
					walk = tree.walker()
				}
			}
		}
	}
	return nil
}
