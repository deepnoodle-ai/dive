package main

import "fmt"

// calculateSum adds two integers and returns their sum
func calculateSum(a, b int) int {
	result := a + b
	fmt.Printf("Adding %d + %d = %d\n", a, b, result)
	return result
}

// calculateProduct multiplies two integers
func calculateProduct(a, b int) int {
	return a * b
}

func main() {
	sum := calculateSum(5, 3)
	product := calculateProduct(5, 3)
	fmt.Printf("Sum: %d, Product: %d\n", sum, product)
}