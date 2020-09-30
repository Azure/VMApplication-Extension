package actionplan

import (
	"fmt"
	"testing"
)

func TestSlice(t *testing.T) {
	s := []int{1, 2, 3, 4, 5, 6, 7}
	for ; len(s) > 0; s = s[1:] {
		e := s[0]
		fmt.Sprintf("Value is %v", e)
	}
}
