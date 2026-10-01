package measure

import "testing"

func TestMedian(t *testing.T) {
	if got := median(nil); got != 0 {
		t.Fatal(got)
	}
	if got := median([]int{9, 1, 4}); got != 4 {
		t.Fatal(got)
	}
	if got := median([]int{8, 2}); got != 8 {
		t.Fatal(got)
	}
}
