package main

import (
	"fmt"
	"time"
)

// TODO: This function could be optimized
func slowFunction() {
	time.Sleep(time.Second * 2)
	fmt.Println("This is slow!")
}

func main() {
	fmt.Println("Hello, World!")
	slowFunction()
}