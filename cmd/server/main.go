package main

import (
	"betterbox"
	"flag"
	"log"
	"os"
)

func main() {
	path := flag.String("directory", "", "Empty directory to write to")
	address := flag.String("address", "localhost", "Network address to listen on")
	port := flag.Int("port", 12345, "TCP port to listen on")
	flag.Parse()
	if *path == "" || *address == "" || *port > 65535 || *port <= 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}
	sv, err := betterbox.NewServer(*address, uint16(*port), *path)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	log.Println("Server: ", sv)
	sv.Listen()
}
