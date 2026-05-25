package detect

import "image/color"

var CocoClasses = [80]string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck",
	"boat", "traffic light", "fire hydrant", "stop sign", "parking meter", "bench",
	"bird", "cat", "dog", "horse", "sheep", "cow", "elephant", "bear", "zebra",
	"giraffe", "backpack", "umbrella", "handbag", "tie", "suitcase", "frisbee",
	"skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
	"skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup",
	"fork", "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
	"broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair", "couch",
	"potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink",
	"refrigerator", "book", "clock", "vase", "scissors", "teddy bear",
	"hair drier", "toothbrush",
}

// palette cycles through 20 visually distinct colors for the 80 COCO classes.
var palette = [20]color.NRGBA{
	{R: 255, G: 56, B: 56, A: 255},
	{R: 255, G: 157, B: 151, A: 255},
	{R: 255, G: 112, B: 31, A: 255},
	{R: 255, G: 178, B: 29, A: 255},
	{R: 207, G: 210, B: 49, A: 255},
	{R: 72, G: 249, B: 10, A: 255},
	{R: 146, G: 204, B: 23, A: 255},
	{R: 61, G: 219, B: 134, A: 255},
	{R: 26, G: 147, B: 52, A: 255},
	{R: 0, G: 212, B: 187, A: 255},
	{R: 44, G: 153, B: 168, A: 255},
	{R: 0, G: 194, B: 255, A: 255},
	{R: 52, G: 69, B: 147, A: 255},
	{R: 100, G: 115, B: 255, A: 255},
	{R: 0, G: 24, B: 236, A: 255},
	{R: 132, G: 56, B: 255, A: 255},
	{R: 82, G: 0, B: 133, A: 255},
	{R: 203, G: 56, B: 255, A: 255},
	{R: 255, G: 149, B: 200, A: 255},
	{R: 255, G: 55, B: 199, A: 255},
}

func classColor(classID int) color.NRGBA {
	return palette[classID%len(palette)]
}
