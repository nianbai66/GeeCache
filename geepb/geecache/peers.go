package geecache

// 传入key选择节点
//
//	type PeerPicker interface {
//		PickerPeer(key string) (peer PeerGetter, ok bool)
//	}
//
// 服务端接口，选择客户端
type Picker interface {
	Pick(key string) (peer Fetcher, ok bool)
}

// 从对应的group里查找缓存值，
// type PeerGetter interface {
// Get(group string, key string) ([]byte, error)
//
//		Get(in *pb.Request, out *pb.Response) error
//	}
//
// 客户端接口，RPC方法请求服务端返回值
type Fetcher interface {
	Fetch(group string, key string) ([]byte, error)
}
