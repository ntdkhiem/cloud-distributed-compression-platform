package main

import (
	"flag"
	"fmt"
	"os"

	"compression-platform/compression"
)

func main() {
	decompFlagPtr := flag.Bool("decode", false, "flag to decode")
	outputFlagPtr := flag.String("output", "output", "flag for naming output file")

	flag.Parse()

	restArgs := flag.Args()

	if len(restArgs) != 1 {
		return
	}

	if *decompFlagPtr {
		payload, err := compression.Decompress(restArgs[0])
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}
		err = os.WriteFile(*outputFlagPtr+".txt", []byte(payload.String()), 0644)
		if err != nil {
			fmt.Println("Failed to write file: ", err)
			return
		}
		fmt.Println("File written successfully to", *outputFlagPtr+".txt")

	} else {
		payload, err := compression.Compress(restArgs[0])
		if err != nil {
			fmt.Println("Error: ", err)
			return
		}
		err = os.WriteFile(*outputFlagPtr+".kn", payload.Bytes(), 0644)
		if err != nil {
			fmt.Println("Failed to write file: ", err)
			return
		}
		fmt.Println("File written successfully to", *outputFlagPtr+".kn")
	}
}
