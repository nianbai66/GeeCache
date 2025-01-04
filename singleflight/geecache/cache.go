package geecache

import (
	"geecache/lru"
	"sync"
)

type cache struct {
	//锁
	mu sync.Mutex
	//lru包外访问lru类型，要大写
	lru *lru.Lru
	//缓存池大小
	cacheBytes int64
}

func (c *cache) add(key string, value ByteView) {
	c.mu.Lock()
	//defer表示add函数结束后，无论是正常结束还是错误结束，都解锁，defer的解锁是压栈方式的解锁，先入后解锁
	defer c.mu.Unlock()
	if c.lru == nil {
		c.lru = lru.New(c.cacheBytes, nil)
	}
	c.lru.Add(key, value)
}

func (c *cache) get(key string) (v ByteView, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {
		return
	} else {
		v, ok := c.lru.Get(key)
		if ok {
			return v.(ByteView), ok
		}
	}
	return
}
