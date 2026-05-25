package detect

import "sort"

// Detection holds a single object detection result in original-image pixel space.
type Detection struct {
	ClassID    int
	ClassName  string
	Confidence float32
	X1, Y1     float32 // top-left
	X2, Y2     float32 // bottom-right
}

// parseOutput decodes YOLOv8 ONNX output into filtered, NMS-applied detections
// mapped to original image space. It handles both common export layouts:
//
//	[1, 4+classes, boxes] — channel-first  (layout.Transposed == false)
//	[1, boxes, 4+classes] — boxes-first    (layout.Transposed == true)
func parseOutput(
	data []float32,
	layout OutputLayout,
	confThresh, nmsThresh float32,
	lb letterboxResult,
	origW, origH int,
) []Detection {
	numBoxes := layout.NumBoxes
	numClasses := layout.NumChannels - 4

	// at returns data[channel, box] regardless of storage layout
	var at func(ch, box int) float32
	if layout.Transposed {
		// data[box * numChannels + ch]
		nc := layout.NumChannels
		at = func(ch, box int) float32 { return data[box*nc+ch] }
	} else {
		// data[ch * numBoxes + box]
		at = func(ch, box int) float32 { return data[ch*numBoxes+box] }
	}

	var candidates []Detection

	for i := 0; i < numBoxes; i++ {
		// find max class score
		bestScore := float32(0)
		bestClass := 0
		for c := 0; c < numClasses; c++ {
			s := at(4+c, i)
			if s > bestScore {
				bestScore = s
				bestClass = c
			}
		}
		if bestScore < confThresh {
			continue
		}

		cx := at(0, i)
		cy := at(1, i)
		w := at(2, i)
		h := at(3, i)

		// convert to corners in letterboxed space
		bx1 := cx - w/2
		by1 := cy - h/2
		bx2 := cx + w/2
		by2 := cy + h/2

		// map back to original image space
		s := lb.scale
		pl := float32(lb.padLeft)
		pt := float32(lb.padTop)
		ox1 := clamp((bx1-pl)/s, 0, float32(origW))
		oy1 := clamp((by1-pt)/s, 0, float32(origH))
		ox2 := clamp((bx2-pl)/s, 0, float32(origW))
		oy2 := clamp((by2-pt)/s, 0, float32(origH))

		if ox2 <= ox1 || oy2 <= oy1 {
			continue
		}

		className := "unknown"
		if bestClass < len(CocoClasses) {
			className = CocoClasses[bestClass]
		}

		candidates = append(candidates, Detection{
			ClassID:    bestClass,
			ClassName:  className,
			Confidence: bestScore,
			X1:         ox1,
			Y1:         oy1,
			X2:         ox2,
			Y2:         oy2,
		})
	}

	return nms(candidates, nmsThresh)
}

// nms applies per-class non-maximum suppression.
func nms(dets []Detection, iouThresh float32) []Detection {
	byClass := make(map[int][]Detection)
	for _, d := range dets {
		byClass[d.ClassID] = append(byClass[d.ClassID], d)
	}

	var result []Detection
	for _, group := range byClass {
		sort.Slice(group, func(i, j int) bool {
			return group[i].Confidence > group[j].Confidence
		})
		suppressed := make([]bool, len(group))
		for i := range group {
			if suppressed[i] {
				continue
			}
			result = append(result, group[i])
			for j := i + 1; j < len(group); j++ {
				if !suppressed[j] && boxIoU(group[i], group[j]) > iouThresh {
					suppressed[j] = true
				}
			}
		}
	}
	return result
}

func boxIoU(a, b Detection) float32 {
	ix1 := max(a.X1, b.X1)
	iy1 := max(a.Y1, b.Y1)
	ix2 := min(a.X2, b.X2)
	iy2 := min(a.Y2, b.Y2)
	inter := max(0, ix2-ix1) * max(0, iy2-iy1)
	aArea := (a.X2 - a.X1) * (a.Y2 - a.Y1)
	bArea := (b.X2 - b.X1) * (b.Y2 - b.Y1)
	return inter / (aArea + bArea - inter + 1e-6)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
