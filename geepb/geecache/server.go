package geecache

import (
	"context"
	"fmt"
	pb "geecache/geecachepb"
	consistenthash "geecache/hash"
	"geecache/registry"
	"google.golang.org/grpc"
	"log"
	"net"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
	"sync"
	"time"
)

// 分布式节点之间的rpc调用的服务端
// 一致性哈希负责去找哪个节点

const (
	defaultAddr     = "127.0.0.1:6324"
	defaultReplicas = 50
)

// 配置了 etcd 客户端的默认设置，包括 etcd 服务的端点地址和拨号超时时间。这是用于服务发现和注册的配置，确保服务器可以与 etcd 集群正确通信。
var (
	defaultEtcdConfig = clientv3.Config{
		Endpoints:   []string{"localhost:2380"},
		DialTimeout: 5 * time.Second,
	}
)

// 服务端，使用etcd客户端向etcd服务端注册服务
type server struct {
	pb.UnimplementedGroupCacheServer //protobuf生成的接口，确保server结构体实现了必须的grpc方法

	addr       string     // format: ip:port
	status     bool       // true: running false: stop
	stopSignal chan error // 通知registry revoke服务
	mu         sync.Mutex
	consHash   *consistenthash.Map
	//每个客户端地址对应一个客户端实例
	clients map[string]*client
}

// NewServer 创建cache的serve 若addr为空 则使用defaultAddr
func NewServer(addr string) (*server, error) {
	if addr == "" {
		addr = defaultAddr
	}
	if !validPeerAddr(addr) {
		return nil, fmt.Errorf("invalid addr %s, it should be x.x.x.x:port", addr)
	}
	return &server{addr: addr}, nil
}

// Get 实现 GoCache service 的 Get 接口
// 根据一致性哈希找到对应得地址，在找到RPC客户端，通过客户端向地址发送RPC调用
// 输入rpc请求返回响应和error
func (s *server) Get(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	// 请求中获取key和缓存池名
	group, key := req.GetGroup(), req.GetKey()
	resp := &pb.Response{}

	log.Printf("[geecache_svr %s] Received RPC Request - Group: %s, Key: %s", s.addr, group, key)
	if key == "" {
		return resp, fmt.Errorf("key is required")
	}

	// 获取缓存池名对应得缓存组，例如score
	g := GetGroup(group)
	if g == nil {
		return resp, fmt.Errorf("group %s not found", group)
	}

	// 尝试从缓存获取数据，组里本地或者远程调用，客户端调用另一个节点得这个服务端
	value, err := g.Get(key)

	if err == nil {
		resp.Value = value.ByteSlice()
		return resp, nil
	}

	// 数据不在缓存中，从数据库加载
	view, err := g.getLocally(key)
	if err != nil {
		return nil, fmt.Errorf("failed to load data for key %s: %v", key, err)
	}

	resp.Value = view.ByteSlice()
	return resp, nil
}

// Start 启动cache服务，对于结构体初始化
// -----------------启动服务----------------------
//  1. 设置status为true 表示服务器已在运行
//  2. 初始化stop channal,这用于通知registry stop keep alive
//  3. 初始化tcp socket并开始监听
//  4. 注册rpc服务至grpc 这样grpc收到request可以分发给server处理
//  5. 将自己的服务名/Host地址注册至etcd 这样client可以通过etcd
//     获取服务Host地址 从而进行通信。这样的好处是client只需知道服务名
//     以及etcd的Host即可获取对应服务IP 无需写死至client代码中
//
// ----------------------------------------------
func (s *server) Start() error {
	s.mu.Lock()
	if s.status == true {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}

	s.status = true
	// 创建一个接收停止信号的通道，这个通道用于从注册服务接收停止或错误信号
	s.stopSignal = make(chan error)
	// 启动TCP服务器，监听指定端口
	port := strings.Split(s.addr, ":")[1]
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	// 创建新的服务器实例
	grpcServer := grpc.NewServer()
	// 这个服务器实例与 gRPC 服务相关联，允许 gRPC 处理到来的请求。
	// 客户端对sever得get请求，gRPC服务器知道调用s中得get方法
	pb.RegisterGroupCacheServer(grpcServer, s)

	// 注册服务至etcd，异步运行服务注册逻辑，避免阻塞主线程
	go func() {
		//注册服务器的地址到etcd，这样客户端可以通过 etcd 发现并连接到这个服务器。
		err := registry.Register("geecache", s.addr, s.stopSignal)
		if err != nil {
			log.Fatalf(err.Error())
		}
		// 注册失败关闭通道，返回了错误信号，主协程就知道了
		close(s.stopSignal)

		err = lis.Close()
		if err != nil {
			log.Fatalf(err.Error())
		}
		log.Printf("[%s] Revoke service and close tcp socket ok.", s.addr)
	}()

	//log.Printf("[%s] register service ok\n", s.addr)
	s.mu.Unlock()

	// 在之前创建的监听器上服务gRPC请求，这是一个阻塞调用，会持续监听直到服务器关闭
	if err := grpcServer.Serve(lis); s.status && err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}
	return nil
}

// SetPeers 将各个远端主机IP添加到Server里
// 这样Server就可以Pick他们了
// 注意: 此操作是*覆写*操作！
// 注意: peersIP必须满足 x.x.x.x:port的格式
func (s *server) SetPeers(peersAddr ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	//初始化一个一致性哈希环
	s.consHash = consistenthash.New(defaultReplicas, nil)
	//供的远程节点地址注册到一致性哈希环中
	s.consHash.Add(peersAddr...)
	//每个节点生成对应得rpc客户端，直接用客户端调用rpc服务
	s.clients = make(map[string]*client)
	for _, peerAddr := range peersAddr {
		if !validPeerAddr(peerAddr) {
			panic(fmt.Sprintf("[peer %s] invalid address format, it should be x.x.x.x:port", peerAddr))
		}
		//对于每一个有效的节点地址，创建并注册新的客户端实例
		service := fmt.Sprintf("gocache/%s", peerAddr)
		//fmt.Sprintln("service is", service)
		s.clients[peerAddr] = NewClient(service)
		// peerAddr -> gocache/peerAddr
		//registry.Register(service,peerAddr,make(chan error, 1))
	}
}

// Pick 根据一致性哈希选举出key应存放在的cache
// return false 代表从本地获取cache
// Fetcher就是客户端实现得接口
func (s *server) Pick(key string) (Fetcher, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	//返回节点地址
	peerAddr := s.consHash.Get(key)
	// Pick itself
	if peerAddr == s.addr {
		log.Printf("ooh! pick myself, I am %s\n", s.addr)
		return nil, false
	}
	log.Printf("[cache %s] pick remote peer: %s\n", s.addr, peerAddr)
	// 返回远程节点
	return s.clients[peerAddr], true

}

// Stop 停止server运行 如果server没有运行 这将是一个no-op
func (s *server) Stop() {
	s.mu.Lock()
	if s.status == false {
		s.mu.Unlock()
		return
	}

	s.stopSignal <- nil // 发送停止keepalive信号
	s.status = false    // 设置server运行状态为stop
	s.clients = nil     // 清空一致性哈希信息 有助于垃圾回收
	s.consHash = nil
	s.mu.Unlock()
}
