package main

import (
	"io"
	"os"
)

func openInput(inputFile string) (io.ReadCloser, error) {
	var ioReader io.ReadCloser
	if inputFile == "-" {
		ioReader = os.Stdin
	} else {
		fp, err := os.Open(inputFile)
		if err != nil {
			return nil, err
		}
		ioReader = fp
	}

	return ioReader, nil
}

func openOutput(outputFile string) (io.WriteCloser, error) {
	var ioWriter io.WriteCloser
	if outputFile == "-" {
		ioWriter = os.Stdout
	} else {
		fp, err := os.Create(outputFile)
		if err != nil {
			return nil, err
		}
		ioWriter = fp
	}

	return ioWriter, nil
}
