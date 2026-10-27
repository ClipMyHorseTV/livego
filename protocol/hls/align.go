package hls

const (
	syncms = 2 // ms
)

type align struct {
	frameNum  uint64
	frameBase uint64
}

func (a *align) align(dts *uint64, inc uint64) {
	aFrameDts := *dts
	estPts := a.frameBase + a.frameNum*inc
	var dPts uint64
	if estPts >= aFrameDts {
		dPts = estPts - aFrameDts
	} else {
		dPts = aFrameDts - estPts
	}

	if dPts <= uint64(syncms)*h264DefaultHz {
		a.frameNum++
		*dts = estPts
		return
	}
	a.frameNum = 1
	a.frameBase = aFrameDts
}
