package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

// 哈希函数类型，接收字节切片，返回int
type Hash func(data []byte) uint32

// map结构体，包含哈希函数和key，和map
type Map struct {
	hash Hash
	//虚拟节点倍数
	replicas int
	keys     []int
	hashMap  map[int]string
}

// 创建一个map
func New(replicas int, fn Hash) *Map {
	m := &Map{
		hash:     fn,
		replicas: replicas,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

// 添加节点，可以接收任意个字符串
func (m *Map) Add(keys ...string) {
	//忽略索引值，数组下标
	for _, key := range keys {
		//每个真实节点名称，创建多个虚拟节点，带有i的虚拟节点名字
		for i := 0; i < m.replicas; i++ {
			//根据i和key值生成哈希key
			hashkey := int(m.hash([]byte(strconv.Itoa(i) + key)))
			m.keys = append(m.keys, hashkey)
			//虚拟节点和真实节点的映射关系
			m.hashMap[hashkey] = key
		}
	}
	sort.Ints(m.keys)
}

// 选择节点的get
func (m *Map) Get(key string) string {
	if len(m.keys) == 0 {
		return ""
	}
	hash := int(m.hash([]byte(key)))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})

	return m.hashMap[m.keys[idx%len(m.keys)]]

}
