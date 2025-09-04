// calculateSum adds two integers and returns their sum
function calculateSum(a, b) {
    const result = a + b;
    console.log(`Adding ${a} + ${b} = ${result}`);
    return result;
}

// calculateProduct multiplies two integers
function calculateProduct(a, b) {
    return a * b;
}

// Main execution
function main() {
    const sum = calculateSum(5, 3);
    const product = calculateProduct(5, 3);
    console.log(`Sum: ${sum}, Product: ${product}`);
}

// Run the main function
main();
