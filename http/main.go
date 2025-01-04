package main

import (
	"fmt"
	"geecache"
	"log"

	"github.com/gin-gonic/gin"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func main() {
	geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(func(key string) ([]byte, error) {
		log.Println("[SlowDB] search key", key)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}))
	// addr := "localhost:9999"
	// peers := geecache.NewHttpPool(addr)
	// log.Println("geecache is running at", addr)
	// log.Fatal(http.ListenAndServe(addr, peers))
	//设置为非debug模式
	gin.SetMode(gin.ReleaseMode)
	addr := "localhost:9999"
	peers := geecache.NewHttpPool(addr)
	log.Println("geecache is running at", addr)

	r := gin.Default()

	r.Any("/*any", func(c *gin.Context) {
		peers.ServeHTTP(c.Writer, c.Request)
	})

	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

}
