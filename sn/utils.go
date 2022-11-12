package sn

import (
	"os"
)

func DirExists(dir string) bool {
	_, err := os.Stat(dir)
	return !os.IsNotExist(err)
}

func MinOf(vars ...int) int {
	min := vars[0]

	for _, i := range vars {
		if min > i {
			min = i
		}
	}

	return min
}
