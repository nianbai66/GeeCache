package geecache

import (
	"context"
	"fmt"
	pb "geecache/geecachepb"
	"geecache/registry"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"log"
	"sync"
	"time"
)

// client 模块实现gocache访问其他远程节点得客户端 从而获取缓存的能力
type client struct {
	name       string // 服务名称 geecache/ip:addr
	etcdClient *clientv3.Client
	conn       *grpc.ClientConn
	mu         sync.Mutex
}

// 初始化客户端，与etcd服务端连接
func (c *client) initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	//clientv3.NewCtxClient()
	//创建etcd客户端
	if c.etcdClient == nil {
		var err error
		c.etcdClient, err = clientv3.New(defaultEtcdConfig)
		if err != nil {
			return fmt.Errorf("failed to create etcd client: %v", err)
		}
	}
	// 客户端根据服务名返回一个rpc连接
	if c.conn == nil {
		conn, err := registry.EtcdDial(c.etcdClient, c.name)
		if err != nil {
			return fmt.Errorf("failed to dial gRPC server: %v", err)
		}
		c.conn = conn
	}

	return nil
}

// 实现fetch接口，
func (c *client) Fetch(group string, key string) ([]byte, error) {
	// 初始化
	if err := c.initialize(); err != nil {
		log.Printf("Initialization failed: %v", err)
		return nil, err
	}
	log.Println("Initialization successful")

	//如果连接成功，会使用这个grpc得连接创建一个新的gRPC客户端
	grpcClient := pb.NewGroupCacheClient(c.conn)
	if grpcClient == nil {
		log.Println("Failed to create gRPC client")
		return nil, fmt.Errorf("failed to create gRPC client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	//发送一个gPRC请求到远程服务，请求包括组名和键名，
	resp, err := grpcClient.Get(ctx, &pb.Request{Group: group, Key: key})
	if err != nil {
		log.Printf("gRPC call failed: %v", err)
		return nil, fmt.Errorf("could not get %s/%s from peer %s: %v", group, key, c.name, err)
	}
	log.Println("Successfully sent gRPC request")
	return resp.GetValue(), nil
}

// 用于创建新的client实例，接收一个服务名作为参数，这个服务名是etcd中注册的服务名，用于在 Fetch 方法中与远程服务通信。
func NewClient(service string) *client {
	// x.x.x.x:port
	return &client{name: service}
}
