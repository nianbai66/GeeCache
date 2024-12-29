package geecache

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

const defaultBasePath = "/_geecache/"

type HTTPPool struct {
	//自己地址
	self string
	//请求的前缀
	basePath string
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
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())
}
