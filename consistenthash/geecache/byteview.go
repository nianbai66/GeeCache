package geecache

type ByteView struct {
	//存储缓存值，byte可以存储任意类型的数据
	b []byte
}

// 缓存ByteView对象必须有len方法
func (v ByteView) Len() int {
	return len(v.b)
}

func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}

func (v ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
