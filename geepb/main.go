package main

import (
	"fmt"
	"geecache"
	"log"
	"sync"
	"time"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func main() {
	// 多个节点的地址
	addrs := []string{"localhost:9999", "localhost:9998", "localhost:9997"}
	groupname := []string{"9999", "9998", "9997"}
	var Group []*geecache.Group
	// 每个节点启动rpc服务
	for i, addr := range addrs {
		// 缓存池包里的创建rpc服务端函数
		svr, err := geecache.NewServer(addr)
		if err != nil {
			// 格式化输出并终止运行
			log.Fatalf("Failed to create server on %s: %v", addr, err)
		}
		// rpc服务里添加其他rpc节点，添加到哈希环里，生成访问对应的客户端
		// 三个真实节点
		svr.SetPeers(addrs...)
		// 多个缓存池节点，有各自的rpc服务
		group := geecache.NewGroup(groupname[i], 2<<10, time.Second, geecache.GetterFunc(
			func(key string) ([]byte, error) {
				log.Println("[Mysql] search key", key)
				if v, ok := db[key]; ok {
					return []byte(v), nil
				}
				return nil, fmt.Errorf("%s not exist", key)
			}))
		// 启动服务
		go func() {
			err := svr.Start()
			if err != nil {
				log.Fatal(err)
			}
		}()

		// rpc服务添加到geecache里
		group.RegisterPeers(svr)
		Group = append(Group, group)

	}
	log.Println("geecache is running at", addrs)
	time.Sleep(5 * time.Second) // 等待服务器启动

	// 发出几个Get请求，分开发送，保证第二次Get缓存命中
	// 可以向任意服务器发起请求
	var wg sync.WaitGroup
	wg.Add(2)
	go GetTomScore(Group[0], &wg)
	go GetJackScore(Group[0], &wg)
	wg.Wait()

	wg.Add(2)
	go GetTomScore(Group[0], &wg)
	go GetJackScore(Group[0], &wg)
	wg.Wait()

	wg.Add(2)
	go GetTomScore(Group[0], &wg)
	go GetJackScore(Group[0], &wg)
	wg.Wait()
}

func GetTomScore(group *geecache.Group, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("get Tom...")
	view, err := group.Get("Tom")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Println(view.String())
}
func GetJackScore(group *geecache.Group, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("get Jack...")
	view, err := group.Get("Jack")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Println(view.String())
}
