// Package clock はフレーム上端に時刻を描いて、デコード後に読む。
package clock

// Bits はミリ秒の UNIX 時刻を載せる幅です。
const Bits = 48

// Paint は RGB24 のフレームへ、左が上位ビットの白黒の時計を描きます。
func Paint(frame []byte, width, height int, unixMilli int64) {
	for i := 0; i+2 < len(frame); i += 3 {
		frame[i] = 0x14
		frame[i+1] = 0x18
		frame[i+2] = 0x14
	}
	if width < Bits || height < 1 {
		return
	}
	bar := float64(width) / Bits
	now := uint64(unixMilli)
	barHeight := 28
	if barHeight > height {
		barHeight = height
	}
	for index := 0; index < Bits; index++ {
		bit := (now >> uint(Bits-1-index)) & 1
		var red, green, blue byte
		if bit == 1 {
			red, green, blue = 255, 255, 255
		}
		x0 := int(float64(index) * bar)
		x1 := int(float64(index+1)*bar) + 1
		if x0 < 0 {
			x0 = 0
		}
		if x1 > width {
			x1 = width
		}
		for y := 0; y < barHeight; y++ {
			for x := x0; x < x1; x++ {
				offset := (y*width + x) * 3
				frame[offset] = red
				frame[offset+1] = green
				frame[offset+2] = blue
			}
		}
	}
}

// ReadStamp は y=8 の各バー中央から時刻を読みます。
func ReadStamp(frame []byte, width int) int64 {
	if width < Bits {
		return 0
	}
	bar := float64(width) / Bits
	var value uint64
	for index := 0; index < Bits; index++ {
		x := int(float64(index)*bar + bar/2)
		if x >= width {
			x = width - 1
		}
		offset := (8*width + x) * 3
		if offset+2 >= len(frame) {
			return 0
		}
		luma := int(frame[offset]) + int(frame[offset+1]) + int(frame[offset+2])
		var bit uint64
		if luma > 380 {
			bit = 1
		}
		value = (value << 1) | bit
	}
	return int64(value)
}
