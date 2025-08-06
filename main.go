package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type HuffLookupItem struct {
	char uint32
	freq int
	code string
	bits int
}

type HuffNode struct {
	value *HuffLookupItem
	Left  *HuffNode
	Right *HuffNode
}

func (hn *HuffNode) AddNode(char uint32, code uint32, bits uint8) {
	if bits == 0 {
		hn.value = &HuffLookupItem{
			char: char,
		}
		return
	}

	direction := code >> (bits - 1)
	if direction&1 == 0 {
		if hn.Left == nil {
			hn.Left = &HuffNode{}
		}
		hn.Left.AddNode(char, code, bits-1)
	} else {
		if hn.Right == nil {
			hn.Right = &HuffNode{}
		}
		hn.Right.AddNode(char, code, bits-1)
	}
}

func (hn *HuffNode) Walker() func(int) (uint32, bool) {
	t := hn
	return func(direction int) (uint32, bool) {
		if direction == 0 {
			t = t.Left
		} else {
			t = t.Right
		}

		if t.value != nil {
			return t.value.char, true
		}
		return 0, false
	}
}

type HuffPriorityQueue []*HuffNode
type TreeTable map[uint32]*HuffLookupItem

func (pq HuffPriorityQueue) Len() int { return len(pq) }

func (pq HuffPriorityQueue) Less(i, j int) bool {
	return pq[i].value.freq > pq[j].value.freq
}

func (pq HuffPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *HuffPriorityQueue) Push(x any) {
	item := x.(*HuffNode)
	*pq = append(*pq, item)
}

func (pq *HuffPriorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*pq = old[0 : n-1]
	return item
}

func BuildHuffmanTree(root *HuffNode, code string, bits int) {
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

	BuildHuffmanTree(root.Left, code+"0", bits+1)
	BuildHuffmanTree(root.Right, code+"1", bits+1)
}

func HuffmanTreeToString(root *HuffNode) {
	if root == nil {
		return
	}

	item := root.value

	if item.char != 0 {
		// print it out here...
		fmt.Printf("%c\t%d\t%s\t%d\n", item.char, item.freq, item.code, item.bits)
	}

	HuffmanTreeToString(root.Left)
	HuffmanTreeToString(root.Right)

}

func ConvertToBin(value uint32) []byte {
	result := make([]byte, 4)
	binary.LittleEndian.PutUint32(result, value)
	return result
}

func ConvertCodeToBin(value string) []byte {
	result := make([]byte, 4)
	v, err := strconv.ParseUint(value, 2, 32)
	if err != nil {
		panic(err)
	}
	smallV := uint32(v)
	binary.LittleEndian.PutUint32(result, smallV)
	return result
}

func BuildHeader(root *HuffNode, header *bytes.Buffer) {
	if root == nil {
		return
	}

	item := root.value

	if item.char != 0 {
		// first four bytes: character code
		_, err := header.Write(ConvertToBin(item.char))
		if err != nil {
			panic(err)
		}
		// next four bytes: Huffman assigned code
		_, err = header.Write(ConvertCodeToBin(item.code))
		if err != nil {
			panic(err)
		}
		// last byte: bits
		err = header.WriteByte(byte(item.bits))
		if err != nil {
			panic(err)
		}
	}

	BuildHeader(root.Left, header)
	BuildHeader(root.Right, header)
}

func BuildBody(pt TreeTable, bodyContent []string, bodyOutput *bytes.Buffer) {
	encodedString := ""
	for _, c := range bodyContent {
		for _, r := range c {
			// TODO: FIX THIS TERRIBLE SOLUTION!
			item := pt[uint32(r)]
			encodedString += item.code
			if len(encodedString)%8 == 0 {
				for i := 0; i < len(encodedString); i += 8 {
					byteString := encodedString[i : i+8]
					val, err := strconv.ParseUint(byteString, 2, 8)
					if err != nil {
						panic(err)
					}
					bodyOutput.WriteByte(byte(val))
				}
				encodedString = ""
			}
		}
	}
	// TODO: IS THERE A BETTER WAY THAN PADDING?
	if len(encodedString)%8 != 0 {
		// pad with extra 0s in the last bits of character
		encodedString += strings.Repeat("0", 8-(len(encodedString)%8))
	}
	for i := 0; i < len(encodedString); i += 8 {
		byteString := encodedString[i : i+8]
		val, err := strconv.ParseUint(byteString, 2, 8)
		if err != nil {
			panic(err)
		}
		bodyOutput.WriteByte(byte(val))
	}
}

func Encode(store map[uint32]int, body []string, EncodedText *bytes.Buffer) {
	fmt.Println("ENCODING...")
	// =========== Build Huffman Priority Queue
	pq := make(HuffPriorityQueue, len(store))
	pt := make(TreeTable)
	i := 0
	for k, v := range store {
		val := &HuffLookupItem{
			char: k,
			freq: -v,
		}
		pt[k] = val
		pq[i] = &HuffNode{
			value: pt[k],
		}
		i++
	}
	heap.Init(&pq)

	for pq.Len() > 1 {
		n1 := heap.Pop(&pq).(*HuffNode)
		n2 := heap.Pop(&pq).(*HuffNode)
		newNode := HuffNode{}
		newNode.value = &HuffLookupItem{
			freq: n1.value.freq + n2.value.freq,
		}
		if n1.value.freq > n2.value.freq {
			newNode.Right = n1
			newNode.Left = n2
		} else {
			newNode.Right = n2
			newNode.Left = n1
		}
		heap.Push(&pq, &newNode)
	}
	BuildHuffmanTree(pq[0], "", 0)

	var Header bytes.Buffer
	BuildHeader(pq[0], &Header)
	fmt.Printf("Header size: %d bytes\n", Header.Len())

	var Body bytes.Buffer
	BuildBody(pt, body, &Body)
	fmt.Printf("Body size: %d bytes\n", Body.Len())

	EncodedText.Grow(2 + Header.Len() + Body.Len())
	headerBin := make([]byte, 2)
	binary.LittleEndian.PutUint16(headerBin, uint16(Header.Len()))
	EncodedText.Write(headerBin)
	fmt.Printf("Write 2 bytes of header length to EncodedText\n")
	n, err := Header.WriteTo(EncodedText)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Write %d bytes of header to EncodedText\n", n)
	n, err = Body.WriteTo(EncodedText)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Write %d bytes of body to EncodedText\n", n)
}

func Decode(text *bytes.Buffer, output *strings.Builder) {
	fmt.Println("Decoding...")
	// Extract header length
	headerLenBin := make([]byte, 2)
	_, err := text.Read(headerLenBin)
	if err != nil {
		panic(err)
	}
	headerLen := binary.LittleEndian.Uint16(headerLenBin)
	fmt.Printf("Extracted header length: %d\n", headerLen)
	// Split text to header and body sections
	headerBin := make([]byte, headerLen)
	_, err = text.Read(headerBin)
	if err != nil {
		panic(err)
	}
	// Build HuffmanTree with extracted header
	ht := HuffNode{}
	// character code + Huffman assigned code + bits -> 4 + 4 + 1 = 9 bytes
	for i := 0; i < len(headerBin); i += 9 {
		section := headerBin[i : i+9]
		char := binary.LittleEndian.Uint32(section[0:4])
		code := binary.LittleEndian.Uint32(section[4:8])
		bits := section[8]
		ht.AddNode(char, code, bits)
	}
	// Decode body using built tree above
	walk := ht.Walker()
	for {
		bodyBin, err := text.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		for i := 7; i >= 0; i-- {
			bit := (bodyBin >> uint(i)) & 1
			v, ok := walk(int(bit))
			if ok {
				output.WriteRune(rune(v))
				walk = ht.Walker()
			}
		}
	}
}

func Compress(filePath string) (*bytes.Buffer, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %v", err)
	}
	defer file.Close()

	store := make(map[uint32]int)
	body := []string{}
	originalSize := 0
	// =========== Build frequency table
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
	var EncodedText bytes.Buffer
	Encode(store, body, &EncodedText)
	fmt.Printf("Total: %d bytes\n", EncodedText.Len())
	return &EncodedText, nil
}

func Decompress(filePath string) (*strings.Builder, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading file: %v", err)
	}

	buf := bytes.NewBuffer(data)
	var DecodedText strings.Builder
	Decode(buf, &DecodedText)
	fmt.Printf("Text size: %d bytes\n", DecodedText.Len())
	return &DecodedText, nil
}

func main() {
	decompFlagPtr := flag.Bool("decode", false, "flag to decode")
	outputFlagPtr := flag.String("output", "output", "flag for naming output file")

	flag.Parse()

	restArgs := flag.Args()

	if len(restArgs) != 1 {
		return
	}

	if *decompFlagPtr {
		payload, err := Decompress(restArgs[0])
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}
		err = os.WriteFile(*outputFlagPtr+".txt", []byte(payload.String()), 0644)
		if err != nil {
			fmt.Println("Failed to write file: ", err)
			return
		}
		fmt.Println("File written successfully to ", *outputFlagPtr+".txt")

	} else {
		payload, err := Compress(restArgs[0])
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}
		err = os.WriteFile(*outputFlagPtr+".kn", payload.Bytes(), 0644)
		if err != nil {
			fmt.Println("Failed to write file: ", err)
			return
		}
		fmt.Println("File written successfully to ", *outputFlagPtr+".kn")
	}
}
