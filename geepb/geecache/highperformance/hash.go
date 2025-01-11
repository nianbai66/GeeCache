package highperformance

import (
	"crypto/md5"
	"geecache/simplelru"
	"math"
	"runtime"
	"sync"
)

// hash中的一个切片有一个缓存池
type HashLruCacheOne struct {
	lru  simplelru.LRUCache
	lock sync.RWMutex
}

// 哈希lru，管理多个lru池
type HashLruCache struct {
	list     []*HashLruCacheOne
	sliceNum int
	size     int
}

// NewHashLRU 构造一个给定大小的LRU
func NewHashLRU(size, sliceNum int) (*HashLruCache, error) {
	return NewHashLruWithEvict(size, sliceNum, nil)
}

// NewHashLruWithEvict 用于在缓存条目被淘汰时的回调函数
func NewHashLruWithEvict(size, sliceNum int, onEvicted func(key interface{}, value interface{}, expirationTime int64)) (*HashLruCache, error) {
	if 0 == sliceNum {
		// 设置为当前cpu数量
		sliceNum = runtime.NumCPU()
	}
	if size < sliceNum {
		size = sliceNum
	}

	// 计算出每个分片的数据长度
	lruLen := int(math.Ceil(float64(size / sliceNum)))
	var h HashLruCache
	h.size = size
	h.sliceNum = sliceNum
	// 切片中每个i创建一个lru缓存池
	h.list = make([]*HashLruCacheOne, sliceNum)
	for i := 0; i < sliceNum; i++ {
		l, _ := simplelru.NewLRU(lruLen, onEvicted)
		h.list[i] = &HashLruCacheOne{
			lru: l,
		}
	}

	return &h, nil
}

// Purge 清除所有缓存项
func (h *HashLruCache) Purge() {
	for i := 0; i < h.sliceNum; i++ {
		h.list[i].lock.Lock()
		h.list[i].lru.Purge()
		h.list[i].lock.Unlock()
	}
}

// PurgeOverdue 用于清除过期缓存。
func (h *HashLruCache) PurgeOverdue() {
	for i := 0; i < h.sliceNum; i++ {
		h.list[i].lock.Lock()
		h.list[i].lru.PurgeOverdue()
		h.list[i].lock.Unlock()
	}
}

// Add 向缓存添加一个值。如果已经存在,则更新信息
func (h *HashLruCache) Add(key interface{}, value interface{}, expirationTime int64) (evicted bool) {
	// 根据key找到哪个切片i
	sliceKey := h.modulus(&key)
	// 对应的lru中add kv
	h.list[sliceKey].lock.Lock()
	evicted = h.list[sliceKey].lru.Add(key, value, expirationTime)
	h.list[sliceKey].lock.Unlock()
	return evicted
}

// Get 从缓存中查找一个键的值。
func (h *HashLruCache) Get(key interface{}) (value interface{}, expirationTime int64, ok bool) {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.Lock()
	value, expirationTime, ok = h.list[sliceKey].lru.Get(key)
	h.list[sliceKey].lock.Unlock()
	return value, expirationTime, ok
}

// Contains 检查某个键是否在缓存中，但不更新缓存的状态
func (h *HashLruCache) Contains(key interface{}) bool {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.RLock()
	containKey := h.list[sliceKey].lru.Contains(key)
	h.list[sliceKey].lock.RUnlock()
	return containKey
}

// Peek 在不更新的情况下返回键值(如果没有找到则返回false),不更新缓存的状态
func (h *HashLruCache) Peek(key interface{}) (value interface{}, expirationTime int64, ok bool) {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.RLock()
	value, expirationTime, ok = h.list[sliceKey].lru.Peek(key)
	h.list[sliceKey].lock.RUnlock()
	return value, expirationTime, ok
}

// ContainsOrAdd 检查键是否在缓存中，而不更新
// 最近或删除它，因为它是陈旧的，如果不是，添加值。
// 返回是否找到和是否发生了驱逐。
func (h *HashLruCache) ContainsOrAdd(key interface{}, value interface{}, expirationTime int64) (ok, evicted bool) {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.Lock()
	defer h.list[sliceKey].lock.Unlock()

	if h.list[sliceKey].lru.Contains(key) {
		return true, false
	}
	// 没有找到，被驱逐了，重新添加
	evicted = h.list[sliceKey].lru.Add(key, value, expirationTime)
	return false, evicted
}

// PeekOrAdd 如果一个key在缓存中，那么这个key就不会被更新
// 最近或删除它，因为它是陈旧的，如果不是，添加值。
// 返回是否找到和是否发生了驱逐，还有值。
func (h *HashLruCache) PeekOrAdd(key interface{}, value interface{}, expirationTime int64) (previous interface{}, ok, evicted bool) {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.Lock()
	defer h.list[sliceKey].lock.Unlock()

	previous, expirationTime, ok = h.list[sliceKey].lru.Peek(key)
	if ok {
		return previous, true, false
	}

	evicted = h.list[sliceKey].lru.Add(key, value, expirationTime)
	return nil, false, evicted
}

// Remove 从缓存中移除提供的键。
func (h *HashLruCache) Remove(key interface{}) (present bool) {
	sliceKey := h.modulus(&key)

	h.list[sliceKey].lock.Lock()
	present = h.list[sliceKey].lru.Remove(key)
	h.list[sliceKey].lock.Unlock()
	return
}

// Resize 调整缓存大小，返回调整前的数量
func (h *HashLruCache) Resize(size int) (evicted int) {
	if size < h.sliceNum {
		size = h.sliceNum
	}

	// 计算出每个分片的数据长度
	lruLen := int(math.Ceil(float64(size / h.sliceNum)))

	for i := 0; i < h.sliceNum; i++ {
		h.list[i].lock.Lock()
		evicted = h.list[i].lru.Resize(lruLen)
		h.list[i].lock.Unlock()
	}
	return evicted
}

// Keys 返回缓存的切片，从最老的到最新的。
func (h *HashLruCache) Keys() []interface{} {

	var keys []interface{}

	allKeys := make([][]interface{}, h.sliceNum)

	// 记录最大的 oneKeys 长度
	var oneKeysMaxLen int

	for s := 0; s < h.sliceNum; s++ {
		h.list[s].lock.RLock()

		if h.list[s].lru.Len() > oneKeysMaxLen {
			oneKeysMaxLen = h.list[s].lru.Len()
		}

		oneKeys := make([]interface{}, h.list[s].lru.Len())
		oneKeys = h.list[s].lru.Keys()
		h.list[s].lock.RUnlock()

		allKeys[s] = oneKeys
	}

	for i := 0; i < h.list[0].lru.Len(); i++ {
		for c := 0; c < len(allKeys); c++ {
			if len(allKeys[c]) > i {
				keys = append(keys, allKeys[c][i])
			}
		}
	}

	return keys
}

// Len 获取缓存已存在的缓存条数
func (h *HashLruCache) Len() int {
	var length = 0

	for i := 0; i < h.sliceNum; i++ {
		h.list[i].lock.RLock()
		length = length + h.list[i].lru.Len()
		h.list[i].lock.RUnlock()
	}
	return length
}

// 根据key获取位于哪个切片i里
func (h *HashLruCache) modulus(key *interface{}) int {
	// key的值转成string
	str := InterfaceToString(*key)
	// 计算string的校验和
	return int(md5.Sum([]byte(str))[0]) % h.sliceNum
}
