package main

import (
	"log"
	"strings"
	"sync"

	"github.com/reBiocoder/zproxy/utils"
)

type InnerProxy struct {
	Id             uint64
	AuthKey        string
	ConnChangeSign chan bool //连接池中的连接有改变
	ConnAddr       string
	ProxyAddr      string
	ConnList       sync.Map
	IdleList       sync.Map
	MaxConnNumber  int
}
type InnerJsonConfig struct {
	InnerAddress  string `json:"innerAddress"` //内网http服务
	ProxyAddress  string `json:"proxyAddress"` //服务器的tcp地址
	AuthKey       string `json:"authKey"`
	MaxConnNumber int    `json:"maxConnNumber"`
}

func (ip *InnerProxy) generateId() uint64 {
	ip.Id++
	if ip.Id > 65535*2 {
		ip.Id = 1
	}
	_, ok := ip.ConnList.Load(ip.Id)
	if ok {
		ip.generateId()
	}
	return ip.Id
}

func (ip *InnerProxy) CheckConnNumber() {
	for range ip.ConnChangeSign {
		idleNum := 0
		ip.IdleList.Range(func(key, value interface{}) bool {
			idleNum++
			in := value.(*utils.InnerConnection)
			if in.Status == utils.StatusProxy { //该连接已经被使用，在空闲队列中删除
				ip.IdleList.Delete(key)
			}
			if in.Status == utils.StatusClose { //该连接已经被关闭，应该在conn队列和idle队列中均删除
				ip.IdleList.Delete(key)
				ip.ConnList.Delete(key)
			}
			return true
		})
		newConnNum := ip.MaxConnNumber - idleNum
		ip.CreateNewConn(newConnNum)
	}
}

func (ip *InnerProxy) CreateNewConn(num int) {
	for i := 0; i < num; i++ {
		cid := ip.generateId()
		ic := &utils.InnerConnection{
			Id:            cid,
			AuthKey:       ip.AuthKey,
			ProxyConnAddr: ip.ProxyAddr,
			ConnAddr:      ip.ConnAddr,
			ConnWriteLock: sync.Mutex{},
			StatusMoniter: func(Id uint64, Status int) {
				switch Status {
				case utils.StatusProxy:
					ip.IdleList.Delete(Id)
				case utils.StatusClose:
					ip.IdleList.Delete(Id)
					ip.ConnList.Delete(Id)
				}
			},
		}
		ip.ConnList.Store(ic.Id, ic)
		ip.IdleList.Store(ic.Id, true)
		//对连接到服务器的tcp连接进行鉴权（中间端口）

		//监听中间端口

	}
}

func main() {
	jc := &InnerJsonConfig{}
	utils.Load("./inner.config.json", jc) //解析json
	proxyAddress, connAddress := jc.ProxyAddress, jc.InnerAddress
	proxySlice := strings.Split(proxyAddress, "://")
	connSlice := strings.Split(connAddress, "://")
	if len(proxySlice) != 2 && len(connSlice) != 2 {
		log.Panicln("proxy address or inner address format error")
	}
	if proxySlice[0] != "tcp" {
		log.Panicln("proxy protocol error")
	}

	ip := &InnerProxy{
		AuthKey:        jc.AuthKey,
		ConnAddr:       connSlice[1],
		ProxyAddr:      proxySlice[1],
		ConnChangeSign: make(chan bool, 10),
		MaxConnNumber:  jc.MaxConnNumber,
	}
	if ip.MaxConnNumber <= 0 { //初始化连接池
		ip.MaxConnNumber = 10
	}
	ip.ConnChangeSign <- true //chan没有缓存，写入之后，没有读出会阻塞
	go ip.CheckConnNumber()
}

func init() {
	log.SetPrefix("[inner]")
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Llongfile)
}
