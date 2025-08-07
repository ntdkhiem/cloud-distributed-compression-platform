package compression

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type lookupItem struct {
	char uint32
	freq int
	code string
	bits int
}

type prefixTable map[uint32]*lookupItem

type node struct {
	value *lookupItem
	left  *node
	right *node
}

func (hn *node) addNode(char uint32, code uint32, bits uint8) {
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

func (hn *node) walker() func(int) (uint32, bool) {
	t := hn
	return func(direction int) (uint32, bool) {
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

func buildHuffmanTree(root *node, code string, bits int) {
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

	buildHuffmanTree(root.left, code+"0", bits+1)
	buildHuffmanTree(root.right, code+"1", bits+1)
}

func convertToBin(value uint32) []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, value)
	return result
}

func buildHeader(root *node) (*bytes.Buffer, error) {
	var header bytes.Buffer
	err := buildHeaderRecursive(root, &header)
	if err != nil {
		return nil, err
	}
	return &header, err
}

func convertCodeToBin(value string) ([]byte, error) {
	result := make([]byte, 4)
	v, err := strconv.ParseUint(value, 2, 32)
	if err != nil {
		return nil, err
	}
	smallV := uint32(v)
	binary.LittleEndian.PutUint32(result, smallV)
	return result, nil
}

func buildHeaderRecursive(root *node, header *bytes.Buffer) error {
	if root == nil {
		return nil
	}

	item := root.value

	if item.char != 0 {
		// first four bytes: character code
		if _, err := header.Write(convertToBin(item.char)); err != nil {
			return fmt.Errorf("Failed to write character to header: %w", err)
		}
		// next four bytes: Huffman assigned code
		huffmanCode, err := convertCodeToBin(item.code)
		if err != nil {
			return fmt.Errorf("Failed to encode Huffman code: %w", err)
		}
		if _, err := header.Write(huffmanCode); err != nil {
			return fmt.Errorf("Failed to write Huffman code to header: %w", err)
		}
		// last byte: bits
		if err = header.WriteByte(byte(item.bits)); err != nil {
			return fmt.Errorf("Failed to write bits to header: %w", err)
		}
	}

	if err := buildHeaderRecursive(root.left, header); err != nil {
		return err
	}
	if err := buildHeaderRecursive(root.right, header); err != nil {
		return err
	}

	return nil
}

func buildBody(pt prefixTable, bodyContent []string) (*bytes.Buffer, uint8) {
	var bodyOutput bytes.Buffer
	var currentByte byte
	var bitCount int

	for _, c := range bodyContent {
		for _, r := range c {
			item := pt[uint32(r)]
			for _, bit := range item.code {
				currentByte <<= 1 // shift left to make space for next bit
				if bit == '1' {
					currentByte |= 1
				}
				bitCount++
				if bitCount == 8 {
					bodyOutput.WriteByte(currentByte)
					currentByte = 0
					bitCount = 0
				}
			}
		}
	}

	// flush the remaining bits (pad with 0s on the right)
	var paddedZeros uint8 = 0

	if bitCount > 0 {
		paddedZeros = uint8(8 - bitCount)
		currentByte <<= paddedZeros // shift remaining bits to fill the byte
		bodyOutput.WriteByte(currentByte)
	}
	return &bodyOutput, paddedZeros
}

func Compress(filePath string) (*bytes.Buffer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %v", err)
	}
	defer file.Close()

	var compressData bytes.Buffer
	store := make(map[uint32]int)
	body := []string{}
	originalSize := 0

	fmt.Println("Building frequency table")
	scanner := bufio.NewReader(file)
	for {
		line, err := scanner.ReadString(byte('\n'))
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading file: %v", err)
		}
		originalSize += len(line)
		body = append(body, line)
		for _, c := range line {
			store[uint32(c)] += 1
		}
		if err == io.EOF {
			break
		}
	}
	fmt.Printf("Original File size: %d bytes\n", originalSize)

	pq := make(priorityQueue, len(store))
	pt := make(prefixTable)

	fmt.Println("Building Huffman Priority Queue")
	i := 0
	for k, v := range store {
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
	fmt.Println("Building Huffman prefix Tree")
	buildHuffmanTree(pq[0], "", 0)

	fmt.Println("Building Header")
	header, err := buildHeader(pq[0])
	if err != nil {
		return nil, fmt.Errorf("Error building header: %w", err)
	}

	fmt.Println("Building Body")
	encodedBody, paddedZeros := buildBody(pt, body)
	fmt.Printf("Body size: %d bytes\tPadded 0s: %d\n", encodedBody.Len(), paddedZeros)

	fmt.Println("Merge Header And Body")
	// header's length + header + body's padded zeros + body
	compressData.Grow(2 + header.Len() + 1 + encodedBody.Len())
	headerBin := make([]byte, 2)
	binary.LittleEndian.PutUint16(headerBin, uint16(header.Len()))
	compressData.Write(headerBin)
	fmt.Println("Write 2 bytes of header length to compressData")
	n, err := header.WriteTo(&compressData)
	if err != nil {
		return nil, fmt.Errorf("Error writing header: %w", err)
	}
	fmt.Printf("Write %d bytes of header to compressData\n", n)
	err = compressData.WriteByte(paddedZeros)
	if err != nil {
		return nil, fmt.Errorf("Error writing body padded 0s: %w", err)
	}
	fmt.Println("Write 1 byte of padded 0s to compressData")
	n, err = encodedBody.WriteTo(&compressData)
	if err != nil {
		return nil, fmt.Errorf("Error writing body: %w", err)
	}
	fmt.Printf("Write %d bytes of body to compressData\n", n)

	fmt.Printf("Total: %d bytes\n", compressData.Len())
	return &compressData, nil
}

func Decompress(filePath string) (*strings.Builder, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading file: %v", err)
	}

	buf := bytes.NewBuffer(data)
	var decompText strings.Builder

	fmt.Println("Decoding...")

	// fmt.Println("Extracting header length")
	headerLenBin := make([]byte, 2)
	_, err = buf.Read(headerLenBin)
	if err != nil {
		return nil, fmt.Errorf("Error extracing header: %w", err)
	}
	headerLen := binary.LittleEndian.Uint16(headerLenBin)
	fmt.Printf("Extracted header length: %d\n", headerLen)

	// fmt.Println("Splitting text to header and body sections")
	headerBin := make([]byte, headerLen)
	_, err = buf.Read(headerBin)
	if err != nil {
		return nil, fmt.Errorf("Error splitting header and body: %w", err)
	}
	// fmt.Println("Building HuffmanTree with extracted header")
	ht := node{}
	// character code + Huffman assigned code + bits -> 4 + 4 + 1 = 9 bytes
	for i := 0; i < len(headerBin); i += 9 {
		section := headerBin[i : i+9]
		char := binary.LittleEndian.Uint32(section[0:4])
		code := binary.LittleEndian.Uint32(section[4:8])
		bits := section[8]
		ht.addNode(char, code, bits)
	}
	// fmt.Println("Extracting number of padded 0s")
	paddedZeros, err := buf.ReadByte()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("Error extracting padded 0s: %w", err)
	}
	fmt.Printf("Extracted number of padded 0s: %d\n", paddedZeros)

	// fmt.Println("Decoding body using built tree above")
	walk := ht.walker()
	for {
		bodyBin, err := buf.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("Error decoding body: %w", err)
		}

		var endByte int
		if buf.Len() == 0 {
			endByte = int(paddedZeros)
		} else {
			endByte = 0
		}

		for i := 7; i >= endByte; i-- {
			bit := (bodyBin >> uint(i)) & 1
			v, ok := walk(int(bit))
			if ok {
				decompText.WriteRune(rune(v))
				walk = ht.walker()
			}
		}
	}

	fmt.Printf("Text size: %d bytes\n", decompText.Len())
	return &decompText, nil
}
