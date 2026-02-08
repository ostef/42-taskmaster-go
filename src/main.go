package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: taskmaster <config.yml>")
		os.Exit(1)
	}
	config, err := parseConfig("test.yml")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%+v\n", *config)
}
