package singleflight

import "sync"

//正在进行或者结束的请求，
type call struct {
	//避免重复
	wg sync.WaitGroup
	//空接口，可以接收任意类型的值
	val interface{}
	err error
}

//管理不同的call请求
type Group struct {
	//保护m的锁
	mu sync.Mutex
	m  map[string]*call
}

//针对相同的 key，无论 Do 被调用多少次，函数 fn 都只会被调用一次，对于多个协程，也是返回了多次值
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if g.m == nil {
		//新建map
		g.m = make(map[string]*call)
	}
	//如果重复请求,阻塞当前协程
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		//延迟处理，提供效率
		c.wg.Wait()
		//处理完了直接返回，对于call类型没有上锁，任意协程可以获得值返回
		return c.val, c.err
	}
	//不是重复请求，建立一个新请求对象
	c := new(call)
	//请求前加锁，有一个新任务执行
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	//一个请求结束
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.val, c.err
}
