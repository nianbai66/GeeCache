package lru

import (
	"container/list"
	"sync"
	"time"
)

type Lru struct {
	mu       sync.Mutex
	maxBytes int64
	nbytes   int64
	//list包中得list类型得指针
	l     *list.List
	cache map[string]*list.Element
	//移除节点时调用得函数
	OnEvicted func(key string, value Value)

	// chan struct{}类型的通道，用于信号传递，
	stopChan chan struct{}
	// 清理缓存的时间间隔
	interval time.Duration
}

// 双向链表得节点
type entry struct {
	key   string
	value Value
	// 时间点类型
	expire time.Time
}

// 接口接收任意类型，只要有len()方法任意类型即可作为value
// L大写才能在别的包里定义这个value类型的结构体
type Value interface {
	Len() int
}

// lru得构造函数
func New(maxBytes int64, onEvicted func(string, Value)) *Lru {
	lru := &Lru{
		maxBytes: maxBytes,
		// 可以同时存多种类型，内部的值是一个接口类型
		l:         list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
		// 间隔一分钟
		interval: time.Minute,
		stopChan: make(chan struct{}),
	}
	// 启动定时器
	go lru.startCleanupTimer()
	return lru

}

// 定期清理过期的缓存项，每次定时器触发时，从链表尾部检查每个节点是否过期，过期则移除
func (c *Lru) startCleanupTimer() {
	// 定义ticker时告诉间隔时间
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	// 无限循环
	for {
		select {
		// 每一分钟接收一个当前时间值
		case <-ticker.C:
			for e := c.l.Back(); e != nil; e = e.Prev() {
				kv := e.Value.(*entry)
				// 过期时间！=0，并且现在时间在过期时间之后
				if !kv.expire.IsZero() && time.Now().After(kv.expire) {
					c.removeElement(e)
				} else {
					break
				}
			}

		case <-c.stopChan:
			return
		}
	}
}

// 新增了，查询节点，如果过期删除
func (c *Lru) Get(key string) (v Value, ok bool) {
	if e, ok := c.cache[key]; ok {
		c.l.MoveToFront(e)
		//对e。value取出所指得list元素然后断言是entry指针类型
		kv := e.Value.(*entry)
		// 如果过期了，删除，返回false
		if !kv.expire.IsZero() && time.Now().After(kv.expire) {
			c.removeElement(e)

			return nil, false
		}
		//返回entry中得Value类型得v
		return kv.value, ok
	}
	return
}

func (c *Lru) RemoveOldest() {
	//指向最后元素得指针
	e := c.l.Back()
	if e != nil {
		//
		c.removeElement(e)
	}

}

// 抽象在lru中删除元素，删除l中，map中，大小再-去
func (c *Lru) removeElement(ele *list.Element) {
	c.l.Remove(ele)
	kv := ele.Value.(*entry)
	delete(c.cache, kv.key)
	c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

// 新增了过期时间
func (c *Lru) Add(key string, v Value, expire time.Duration) {
	// 从now到过期时间端之后的时间点
	expires := time.Now().Add(expire)
	if expire == 0 {
		expires = time.Time{} // Set zero time for no expiration
	}

	e, ok := c.cache[key]
	if ok {
		kv := e.Value.(*entry)
		if c.OnEvicted != nil {
			c.OnEvicted(key, kv.value)
		}
		c.l.MoveToFront(e)
		// 修改节点过期时间
		kv.expire = expires
		c.nbytes += int64(v.Len()) - int64(kv.value.Len())
		kv.value = v
	} else {
		e := c.l.PushFront(&entry{key, v, expires})
		c.cache[key] = e
		c.nbytes += int64(v.Len()) + int64(len(key))
	}
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}
func (c *Lru) Len() int {
	return c.l.Len()
}

// 新增删除所有节点
func (c *Lru) Clear() {
	if c.OnEvicted != nil {
		for _, e := range c.cache {
			kv := e.Value.(*entry)
			c.OnEvicted(kv.key, kv.value)
		}
	}
	// 不再指向对象，GC自动回收
	c.l = nil
	c.cache = nil
}

func (c *Lru) Stop() {
	// 关闭通道，发送停止信号让定时器停止
	close(c.stopChan)
}
