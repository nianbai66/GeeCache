package registry

import (
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"log"
)

// EtcdDial 向grpc请求一个服务，用于连接到一个通过etcd注册的grpc服务，该函数会使用etcd作为服务发现机制，
// 通过etcd获取gRPC服务的地址，并建立连接。
// 通过提供一个etcd client和service name即可获得Connection
func EtcdDial(c *clientv3.Client, service string) (*grpc.ClientConn, error) {
	//c：一个创建好的etcd客户端，用于服务发现，service:需要连接的服务名称。返回一个gprc客户端连接和一个可能的错误
	//使用传入的etcd客户端创建一个etcd解析器
	etcdResolver, err := resolver.NewBuilder(c)
	log.Println("Trying to dial etcd with service name:", service)
	if err != nil {
		return nil, err
	}

	//通过etcd解析服务名，通过服务名和这个ip+端口建立Grpc连接，返回建立的gRPC连接
	//第一个参数 "etcd:///"+service：指定要连接的服务名称，这里使用 etcd 解析器来解析服务地址。"etcd:///" 是 etcd 解析器的 URI 前缀，后面接服务名称。
	conn, err := grpc.Dial("etcd:///"+service,
		//grpc.WithResolvers(etcdResolver)：设置 gRPC 解析器为刚才创建的 etcd 解析器。
		grpc.WithResolvers(etcdResolver),
		//grpc.WithInsecure()：不使用 SSL/TLS 进行加密。这通常在开发和测试环境中使用
		grpc.WithInsecure(),
		grpc.FailOnNonTempDialError(true), // Fail fast on permanent errors
	)
	if err != nil {
		log.Printf("Failed to connect to service: %v", err)
		return nil, err
	}
	return conn, err
}
