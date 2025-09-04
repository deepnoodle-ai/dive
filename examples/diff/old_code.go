package main

import "fmt"

func calculateSum(a, b int) int {
	return a + b
}

func main() {
	result := calculateSum(5, 3)
	fmt.Println("Result:", result)
}