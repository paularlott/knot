package util

import "fmt"

func PrintTable(table [][]string) {
	// Find the maximum width of each column
	maxWidths := make([]int, len(table[0]))
	for _, row := range table {
		for i, cell := range row {
			if len(cell) > maxWidths[i] {
				maxWidths[i] = len(cell)
			}
		}
	}

	// Print each row
	for _, row := range table {
		for i, cell := range row {
			// Pad the columns as necessary
			fmt.Printf("%-*s  ", maxWidths[i], cell)
		}
		fmt.Println()
	}
}
