package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	"geecache/singleflight"
	"log"
	"sync"
)

// 接口
type Getter interface {
	Get(key string) ([]byte, error)
}

// 定义的函数类型：这样的回调函数，是GetterFUnc类型
type GetterFunc func(key string) ([]byte, error)

// 这个类型构造时传入一个这样的函数，然后Get属于这个类型，调用这个类型其中的函数
// GetterFunc函数类型，实现了接口的方法，接口型函数
// 只要重写了Getter中的Get方法的任意类型，都可以转换成GetterFunc，作为参数
func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	//不同缓存池用不同名字
	name string
	//缓存未命中时获取源数据的回调函数
	getter Getter
	//本地主缓存
	mainCache cache
	//选择节点
	peers PeerPicker
	//每个key只访问一次
	loader *singleflight.Group
}

// 两个全局变量，锁和多个单机缓存池的map
var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:   name,
		getter: getter,
		//构造一个单机缓存池只用传缓存池大小
		mainCache: cache{cacheBytes: cacheBytes},
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	return g
}

func GetGroup(name string) *Group {
	mu.RLock()
	defer mu.RUnlock()
	g := groups[name]
	return g
}

// s实现了 PeerPicker 接口的 HTTPPool 注入到 Group 中
// 为创建的缓存池注册一个PeerPicker（选择节点） 实例，就可以在本地找不到时，选择服务器
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

// group中的get方法
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}
	//找到
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[Geechche] hit")
		return v, nil
	}
	//没有
	return g.load(key)

}

// 使用 PickPeer() 方法选择节点，若非本机节点，则调用 getFromPeer() 从远程获取。若是本机节点或失败，则回退到 getLocally()。
func (g *Group) load(key string) (value ByteView, err error) {
	//使用do函数，让key只去查询一次远程和获取一次远程的值
	view, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			if peer, ok := g.peers.PickerPeer(key); ok {
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}
		//本地去获取db并缓存到本地
		return g.getLocally(key)
	})
	if err == nil {
		return view.(ByteView), nil
	}
	return
}

// 使用实现了 PeerGetter 接口的 httpGetter 从访问远程节点，获取缓存值
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
	// bytes, err := peer.Get(g.name, key)
	// if err != nil {
	// 	return ByteView{}, err
	// }
	// return ByteView{b: bytes}, nil
}

// 未命中从数据源的get中获取key的值
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	//获取成功克隆一份，对于ByteView这个类型的值的操作，都在ByteView文件里
	//同一个包可以调用函数，从db取数据要深拷贝一份
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil

}

// 添加到主缓存池中
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}
