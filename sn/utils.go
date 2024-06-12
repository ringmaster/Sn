package sn

import (
	"fmt"
	"os"
)

func DirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func MinOf(vars ...int) int {
	min := vars[0]

	for _, i := range vars {
		if i < min {
			min = i
		}
	}

	return min
}

func MaxOf(vars ...int) int {
	max := vars[0]

	for _, i := range vars {
		if i > max {
			max = i
		}
	}

	return max
}

func CopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range m {
		_, ok := v.(map[string]interface{})
		if !ok {
			cp[k] = v
		}
	}

	return cp
}

func PrintMap(data map[string]interface{}, indent string) string {
	output := ""
	for key, value := range data {
		// Check if the value is a nested map
		if nestedMap, ok := value.(map[string]interface{}); ok {
			output += fmt.Sprintf("%sKey: %s, Value: \n", indent, key)
			PrintMap(nestedMap, indent+"  ") // Recursive call with increased indentation
		} else {
			output += fmt.Sprintf("%sKey: %s, Value: %v\n", indent, key, value)
		}
	}
	return output
}
