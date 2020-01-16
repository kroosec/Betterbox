package main

import (
	"betterbox"
	"flag"
	"log"
	"os"
)

func main() {
	path := flag.String("directory", "", "Directory to monitor and update")
	address := flag.String("address", "localhost", "Network address to listen on")
	port := flag.Int("port", 12345, "TCP port to listen on")
	flag.Parse()
	if *path == "" || *address == "" || *port > 65535 || *port <= 0 {
		flag.PrintDefaults()
		os.Exit(0)
	}
	cl, err := betterbox.NewClient(*address, uint16(*port), *path)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	defer cl.Close()
	if err := cl.SyncAndMonitor(); err != nil {
		log.Println(err)
	}
}
