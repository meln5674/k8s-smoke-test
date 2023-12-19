package main

import (
	"log"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"
)

var (
	filePath     = flag.String("rwx-volume-mount", "/var/lib/k8s-smoke-test/rwx", "Path the RWX volume was mounted to")
	fileName     = flag.String("file-name", "test-file", "Name of the file to write to the RWX volume mount")
	fileContents = flag.String("file-contents", "This is a test file", "Contents to write to the test file")
)

func main() {
	flag.Parse()

	f, err := os.Create(filepath.Join(*filePath, *fileName))
	if err != nil {
		log.Fatal(err)
	}
	_, err = f.Write([]byte(*fileContents))
	if err != nil {
		log.Fatal(err)
	}
}
