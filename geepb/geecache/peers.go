package geecache

import pb "geecache/geecachepb"

// 传入key选择节点
type PeerPicker interface {
	PickerPeer(key string) (peer PeerGetter, ok bool)
}

// 从对应的group里查找缓存值，
type PeerGetter interface {
	//Get(group string, key string) ([]byte, error)
	Get(in *pb.Request, out *pb.Response) error
}
