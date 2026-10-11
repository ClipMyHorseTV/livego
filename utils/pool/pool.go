package pool

import "sync"

const maxpoolsize = 500 * 1024

var bytePool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, maxpoolsize)
		return &b
	},
}

func Get(size int) []byte {
	if size > maxpoolsize {
		return make([]byte, size)
	}
	ptr := bytePool.Get().(*[]byte)
	b := *ptr
	return b[:size]
}

func Put(b []byte) {
	if cap(b) != maxpoolsize {
		return
	}
	b = b[:cap(b)]
	bytePool.Put(&b)
}
