package websocket

import (
	"errors"
	"math/rand"
	"sort"
	"sync/atomic"
)

// ClientSelector 客户端选择器接口
type ClientSelector interface {
	Select(clients map[*Client]bool) (*Client, error)
}

// RoundRobinSelector 轮询选择器
type RoundRobinSelector struct {
	counter uint64
}

// NewRoundRobinSelector 创建轮询选择器
func NewRoundRobinSelector() *RoundRobinSelector {
	return &RoundRobinSelector{}
}

// Select 轮询选择客户端
func (s *RoundRobinSelector) Select(clients map[*Client]bool) (*Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("no available client")
	}

	// 将 map 转换为 slice
	clientList := make([]*Client, 0, len(clients))
	for c := range clients {
		clientList = append(clientList, c)
	}
	sort.Slice(clientList, func(i, j int) bool {
		return clientList[i].ID < clientList[j].ID
	})

	// 原子递增计数器并取模
	idx := atomic.AddUint64(&s.counter, 1) - 1
	return clientList[idx%uint64(len(clientList))], nil
}

// LeastConnectionSelector 最少连接选择器
type LeastConnectionSelector struct{}

// NewLeastConnectionSelector 创建最少连接选择器
func NewLeastConnectionSelector() *LeastConnectionSelector {
	return &LeastConnectionSelector{}
}

// Select 选择活跃请求最少的客户端
func (s *LeastConnectionSelector) Select(clients map[*Client]bool) (*Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("no available client")
	}

	var selected *Client
	minLoad := int(^uint(0) >> 1) // MaxInt

	for c := range clients {
		load := c.GetActiveRequests()
		if load < minLoad {
			minLoad = load
			selected = c
		}
	}

	if selected == nil {
		return nil, errors.New("no available client")
	}

	return selected, nil
}

// WeightedSelector 加权选择器
type WeightedSelector struct {
	weights map[string]int // clientID -> weight
}

// NewWeightedSelector 创建加权选择器
func NewWeightedSelector(weights map[string]int) *WeightedSelector {
	if weights == nil {
		weights = make(map[string]int)
	}
	return &WeightedSelector{
		weights: weights,
	}
}

// Select 根据权重选择客户端
func (s *WeightedSelector) Select(clients map[*Client]bool) (*Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("no available client")
	}

	// 计算总权重
	totalWeight := 0
	for c := range clients {
		weight := s.getWeight(c.ID)
		if weight > 0 {
			totalWeight += weight
		}
	}

	// 如果没有配置权重，使用默认权重 1
	if totalWeight == 0 {
		for range clients {
			totalWeight++
		}
	}

	// 随机选择
	random := rand.Intn(totalWeight)
	for c := range clients {
		weight := s.getWeight(c.ID)
		if weight <= 0 {
			weight = 1 // 默认权重
		}
		if random < weight {
			return c, nil
		}
		random -= weight
	}

	// 理论上不会到这里，但为了安全返回第一个
	for c := range clients {
		return c, nil
	}

	return nil, errors.New("no available client")
}

// getWeight 获取客户端权重
func (s *WeightedSelector) getWeight(clientID string) int {
	if weight, ok := s.weights[clientID]; ok {
		return weight
	}
	return 1 // 默认权重
}

// SetWeight 设置客户端权重
func (s *WeightedSelector) SetWeight(clientID string, weight int) {
	s.weights[clientID] = weight
}

// RandomSelector 随机选择器
type RandomSelector struct{}

// NewRandomSelector 创建随机选择器
func NewRandomSelector() *RandomSelector {
	return &RandomSelector{}
}

// Select 随机选择客户端
func (s *RandomSelector) Select(clients map[*Client]bool) (*Client, error) {
	if len(clients) == 0 {
		return nil, errors.New("no available client")
	}

	// 将 map 转换为 slice
	clientList := make([]*Client, 0, len(clients))
	for c := range clients {
		clientList = append(clientList, c)
	}

	// 随机选择
	idx := rand.Intn(len(clientList))
	return clientList[idx], nil
}
