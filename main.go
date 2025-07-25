package main

import (
    "fmt"
    "os"
    "io"
    "bufio"
    "container/heap"
)

type HuffLookupItem struct {
    char    int32
    freq    int
    code    string
    bits    int
}

type HuffNode struct {
    value   *HuffLookupItem
    Left    *HuffNode
    Right   *HuffNode
}

type HuffPriorityQueue []*HuffNode

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
    
    BuildHuffmanTree(root.Left, code + "0", bits + 1)
    BuildHuffmanTree(root.Right, code + "1", bits + 1)
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

func Encode(store map[int32]int) {
    fmt.Println("ENCODING...")
    // =========== Build Huffman Priority Queue
    pq := make(HuffPriorityQueue, len(store))
    i := 0
    for k, v := range store {
        val := &HuffLookupItem{
            char:   k,
            freq:   -v,
        }
        pq[i] = &HuffNode{
            value:  val,
        }
        i++
    }
    heap.Init(&pq)
    
    for pq.Len() > 1 {
        n1 := heap.Pop(&pq).(*HuffNode)
        n2 := heap.Pop(&pq).(*HuffNode)
        // fmt.Printf("n1: %b, %d\nn2: %b, %d\n", n1.value.char, n1.value.freq, n2.value.char, n2.value.freq)
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
    // =========== Build Huffman Tree
    BuildHuffmanTree(pq[0], "", 0)
    HuffmanTreeToString(pq[0])
}

func main() {
    // =========== Take in file's path as argument
    filePath := os.Args[1]

    file, err := os.Open(filePath)
    if err != nil {
        panic(err)
    }
    defer file.Close()

    store := make(map[int32]int)
    body := []string{}
    originalSize := 0

    // =========== Build frequency table
    scanner := bufio.NewReader(file)
    for {
        line, err := scanner.ReadString(byte('\n'))
        if err != nil && err != io.EOF {
            panic(err)
        }
        originalSize += len(line)
        body = append(body, line)
        for _, c := range line {
            // fmt.Printf("%c, %T, %b\n", c, c, c)
            store[c] += 1
        }
        if err == io.EOF {
            break
        }
    }
    // fmt.Printf("store len: %d\n", len(store))
    // for k, v := range store {
    //     fmt.Printf("%c: %d\n", k, v)
    // }
    fmt.Printf("Original File size: %d bytes\n", originalSize)
    fmt.Println("======================")
    Encode(store)
}
