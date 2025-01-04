package geecache

import (
	"fmt"
	pb "geecache/geecachepb"
	consistenthash "geecache/hash"
	"github.com/golang/protobuf/proto"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultReplicas = 50
	defaultBasePath = "/_geecache/"
)

type HTTPPool struct {
	//自己地址
	self string
	//请求的前缀
	basePath string
	mu       sync.Mutex
	//8001服务端的map,每个端口都有几个虚拟节点
	peers *consistenthash.Map
	//每个getter对应一个baseurl，每个getter只能向固定的baseurl发送请求
	httpGetters map[string]*httpGetter
}

func NewHttpPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

// log函数，format格式化字符串（带有%s占位符的），v是参数
func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Serves %s] %s", p.self, fmt.Sprintf(format, v...))
}

// 服务端接收客户端的get请求，
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//路径前缀是否为base
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		panic("HTTPPool serving unexpected path: " + r.URL.Path)
	}
	//格式为/<basepath>/<groupname>/<key>
	p.Log("%s %s", r.Method, r.URL.Path)
	//从basepath后面取出按/划分的字符串，是两个
	//切片操作，从/<basepath>/<groupname>/<key>中的len开始到结尾的切片
	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	groupName := parts[0]
	key := parts[1]
	//获取缓存池
	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//get值后序列化程proto的消息
	body, err := proto.Marshal(&pb.Response{Value: view.ByteSlice()})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body)
}

// 为每个服务器设置虚拟节点，每个服务端设置他的请求客户端对象
func (p *HTTPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	//8001建立一致性哈希map，根据哈希值知道去哪个服务器找
	p.peers = consistenthash.New(defaultReplicas, nil)
	//将所有服务器add到8001map里
	p.peers.Add(peers...)
	//每个服务器创建客户端，记录到8001里
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		//每个节点创建一个客户端，访问地址为
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

// 选择服务器
func (p *HTTPPool) PickerPeer(key string) (peer PeerGetter, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	//根据一致性哈希，根据key选择就近的服务器
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		//返回一致算法选出的url字符串对应的getter
		return p.httpGetters[peer], true
	}
	return nil, false
}

var _ PeerPicker = (*HTTPPool)(nil)

// 客户端，
type httpGetter struct {
	baseURL string
}

// getter的get方法，向指定URL发送httpget请求
func (h *httpGetter) Get(in *pb.Request, out *pb.Response) error {
	//访问的地址
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(in.GetGroup()),
		url.QueryEscape(in.GetKey()),
	)
	//get请求访问服务端
	res, err := http.Get(u)
	if err != nil {
		return err
	}
	//关闭连接
	defer res.Body.Close()
	//状态码不对
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}
	//读数据
	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}
	if err = proto.Unmarshal(bytes, out); err != nil {
		return fmt.Errorf("decoding response body: %v", err)
	}
	return nil

}

var _ PeerGetter = (*httpGetter)(nil)
