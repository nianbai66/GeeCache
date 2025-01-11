package registry

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"log"
	"time"
)

// register模块提供服务Service注册至etcd的能力
var (
	defaultEtcdConfig = clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	}
)

// etcdAdd 在租赁模式添加一对kv（服务名和地址）至etcd
// 租约lid到期，端点信息删除
func etcdAdd(c *clientv3.Client, lid clientv3.LeaseID, service string, addr string) error {
	//用于管理与 service 相关的 etcd 端点数据
	em, err := endpoints.NewManager(c, service)
	if err != nil {
		return err
	}
	//return em.AddEndpoint(c.Ctx(), service+"/"+addr, endpoints.Endpoint{Addr: addr})
	//将一个新的端点添加到 etcd 中。"gocache/127.0.0.1:8080"
	// c.Ctx() 获取与 etcd 客户端 c 关联的上下文，用于控制该操作的生命周期，例如设置超时或取消操作。
	// clientv3.WithLease(lid) 是一个选项，将添加的端点与给定的租约 lid 关联起来。
	return em.AddEndpoint(c.Ctx(), service+"/"+addr, endpoints.Endpoint{Addr: addr}, clientv3.WithLease(lid))
}

// Register 注册一个服务至etcd，不同端口是不同服务
// 注意 Register将不会return 如果没有error的话
func Register(service string, addr string, stop chan error) error {
	// 创建一个etcd client
	cli, err := clientv3.New(defaultEtcdConfig)
	if err != nil {
		return fmt.Errorf("create etcd client failed: %v", err)
	}
	defer cli.Close()
	// 创建一个租约 配置5秒过期
	resp, err := cli.Grant(context.Background(), 5)
	if err != nil {
		return fmt.Errorf("create lease failed: %v", err)
	}
	leaseId := resp.ID
	// 注册服务
	err = etcdAdd(cli, leaseId, service, addr)
	if err != nil {
		return fmt.Errorf("add etcd record failed: %v", err)
	}
	// 设置服务心跳检测
	ch, err := cli.KeepAlive(context.Background(), leaseId)
	if err != nil {
		return fmt.Errorf("set keepalive failed: %v", err)
	}

	log.Printf("[%s] register service ok\n", addr)
	for {
		select {
		// 停止服务注册
		case err := <-stop:
			if err != nil {
				log.Println(err)
			}
			return err
			//客户端关闭
		case <-cli.Ctx().Done():
			log.Println("service closed")
			return nil
			// 返回false，租约撤销，
		case _, ok := <-ch:
			// 监听租约
			if !ok {
				log.Println("keep alive channel closed")
				_, err := cli.Revoke(context.Background(), leaseId)
				return err
			}
			//log.Printf("Recv reply from service: %s/%s, ttl:%d", service, addr, resp.TTL)
		}
	}
}
