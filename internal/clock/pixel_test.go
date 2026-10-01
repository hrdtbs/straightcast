package clock

import "testing"

func TestPixelClockRoundTrip(t *testing.T) {
	const width, height = 640, 352
	frame := make([]byte, width*height*3)
	for _, stamp := range []int64{1_700_000_000_000, 1_758_000_123_456, 42} {
		Paint(frame, width, height, stamp)
		if got := ReadStamp(frame, width); got != stamp {
			t.Fatalf("stamp %d read as %d", stamp, got)
		}
	}
}
