package main

import (
	"fmt"
	"log"

	"gator/internal/config"
)

func main() {
	// 1) Read the config file
	cfg, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}

	// 2) Set current user and write it back to disk
	// Use your name instead of "lane"
	if err := cfg.SetUser("avery"); err != nil {
		log.Fatal(err)
	}

	// 3) Read again and print
	cfg2, err := config.Read()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%+v\n", cfg2)
}
