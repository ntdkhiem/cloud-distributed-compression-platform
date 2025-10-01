package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// type Content struct {
//     huffmanNode
// }
//
// type Server struct {
// 	mu     sync.Mutex
//     sessions map[string]
// }

func main() {
	fmt.Println("Listening on localhost:8080...")
	http.ListenAndServe(":8080", nil)
}

func newUIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
	}
}
