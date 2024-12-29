package lru

import (
	"container/list"
)

type Lru struct {
	maxBytes int64
	nbytes   int64
	//list包中得list类型得指针
	l     *list.List
	cache map[string]*list.Element
	//移除节点时调用得函数
	OnEvicted func(key string, value Value)
}

// 双向链表得节点
type entry struct {
	key   string
	value Value
}

// 接口接收任意类型，只要有len()方法任意类型即可作为value
// L大写才能在别的包里定义这个value类型的结构体
type Value interface {
	Len() int
}

// lru得构造函数
func New(maxBytes int64, onEvicted func(string, Value)) *Lru {
	return &Lru{
		maxBytes:  maxBytes,
		l:         list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}
func (c *Lru) Get(key string) (v Value, ok bool) {
	if e, ok := c.cache[key]; ok {
		c.l.MoveToFront(e)
		//对e。value取出所指得list元素然后断言是entry指针类型
		kv := e.Value.(*entry)
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
		kv := e.Value.(*entry)
		//map得内置删除
		delete(c.cache, kv.key)
		c.l.Remove(e)
		//删除key和值得字节
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			c.OnEvicted(kv.key, kv.value)
		}
	}

}
func (c *Lru) Add(key string, v Value) {
	e, ok := c.cache[key]
	if ok {
		c.l.MoveToFront(e)
		kv := e.Value.(*entry)
		c.nbytes += int64(v.Len()) - int64(kv.value.Len())
		kv.value = v
	} else {
		e := c.l.PushFront(&entry{key, v})
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
