package main

import (
	"container/heap"
	"log"
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
