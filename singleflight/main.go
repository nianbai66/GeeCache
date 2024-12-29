package main

import (
	"flag"
	"fmt"
	"geecache"
	"log"
	"net/http"
)

// 数据
var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func createGroup() *geecache.Group {
	return geecache.NewGroup("score", 2<<10, geecache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

// 启动缓存服务器，创建httppool，添加节点信息，注册到gee中，启动http服务，3个端口
func startCacheServer(addr string, addrs []string, gee *geecache.Group) {
	//建立一个http池，这个池重写了PickerPeer可以选择服务器
	peers := geecache.NewHttpPool(addr)
	//池里添加所有服务端，以便选择
	peers.Set(addrs...)
	//和缓存池关联
	gee.RegisterPeers(peers)
	log.Println("geecache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}

// 另一个端口启动API服务，与用户交互，用score数据的缓存池
func startAPIServer(apiAddr string, gee *geecache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			//查询调用score缓存池的get
			view, err := gee.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())
		}))
	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}
func main() {
	var port int
	var api bool
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.BoolVar(&api, "api", false, "start a api server?")
	flag.Parse()

	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	startCacheServer(addrMap[port], []string(addrs), gee)

	// geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(func(key string) ([]byte, error) {
	// 	log.Println("[SlowDB] search key", key)
	// 	if v, ok := db[key]; ok {
	// 		return []byte(v), nil
	// 	}
	// 	return nil, fmt.Errorf("%s not exist", key)
	// }))
	// // addr := "localhost:9999"
	// // peers := geecache.NewHttpPool(addr)
	// // log.Println("geecache is running at", addr)
	// // log.Fatal(http.ListenAndServe(addr, peers))
	// //设置为非debug模式
	// gin.SetMode(gin.ReleaseMode)
	// addr := "localhost:9999"
	// peers := geecache.NewHttpPool(addr)
	// log.Println("geecache is running at", addr)

	// r := gin.Default()

	// r.Any("/*any", func(c *gin.Context) {
	// 	peers.ServeHTTP(c.Writer, c.Request)
	// })

	// if err := r.Run(addr); err != nil {
	// 	log.Fatalf("Failed to start server: %v", err)
	// }

}
