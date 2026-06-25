package loadgen

import "sort"

// BlockMedians groups sequential RTT samples into fixed-size blocks and returns each block median.
// blockSize is typically RPS (one second of steady traffic).
func BlockMedians(rtts []int64, blockSize int) []float64 {
	if blockSize <= 0 {
		blockSize = 1
	}
	out := make([]float64, 0, len(rtts)/blockSize+1)
	for i := 0; i < len(rtts); i += blockSize {
		end := i + blockSize
		if end > len(rtts) {
			end = len(rtts)
		}
		block := rtts[i:end]
		cp := make([]int64, len(block))
		copy(cp, block)
		sort.Slice(cp, func(a, b int) bool { return cp[a] < cp[b] })
		mid := len(cp) / 2
		if len(cp)%2 == 0 && len(cp) > 0 {
			out = append(out, float64(cp[mid-1]+cp[mid])/2)
		} else if len(cp) > 0 {
			out = append(out, float64(cp[mid]))
		}
	}
	return out
}
