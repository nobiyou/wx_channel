package websocket

import (
	"sync"
	"testing"
)

// 创建测试用的客户端
func createTestClient(id string, activeRequests int) *Client {
	return &Client{
		ID:             id,
		activeRequests: int32(activeRequests),
		send:           make(chan []byte, 256),
		Conn:           nil, // 测试不需要真实连接
	}
}

// TestRoundRobinSelector 测试轮询选择器
func TestRoundRobinSelector(t *testing.T) {
	selector := NewRoundRobinSelector()

	// 创建 3 个客户端
	client1 := createTestClient("client1", 0)
	client2 := createTestClient("client2", 0)
	client3 := createTestClient("client3", 0)

	clients := map[*Client]bool{
		client1: true,
		client2: true,
		client3: true,
	}

	// 测试轮询分布
	counts := make(map[string]int)
	for i := 0; i < 300; i++ {
		client, err := selector.Select(clients)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[client.ID]++
	}

	// 验证分布均匀（每个客户端应该被选中约 100 次，允许 10% 误差）
	expectedCount := 100
	tolerance := 10
	for id, count := range counts {
		if count < expectedCount-tolerance || count > expectedCount+tolerance {
			t.Errorf("Client %s selected %d times, expected ~%d (±%d)", id, count, expectedCount, tolerance)
		}
	}
}

func TestRoundRobinSelectorStableAcrossFreshMaps(t *testing.T) {
	selector := NewRoundRobinSelector()

	for i := 0; i < 60; i++ {
		clients := map[*Client]bool{
			createTestClient("client1", 0): true,
			createTestClient("client2", 0): true,
			createTestClient("client3", 0): true,
		}

		selected, err := selector.Select(clients)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}

		expected := "client" + string('1'+rune(i%3))
		if selected.ID != expected {
			t.Fatalf("iteration %d: expected %s, got %s", i, expected, selected.ID)
		}
	}
}

// TestLeastConnectionSelector 测试最少连接选择器
func TestLeastConnectionSelector(t *testing.T) {
	selector := NewLeastConnectionSelector()

	// 创建 3 个客户端，活跃请求数不同
	client1 := createTestClient("client1", 5)
	client2 := createTestClient("client2", 2)
	client3 := createTestClient("client3", 8)

	clients := map[*Client]bool{
		client1: true,
		client2: true,
		client3: true,
	}

	// 应该选择 client2（活跃请求最少）
	selected, err := selector.Select(clients)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}

	if selected.ID != "client2" {
		t.Errorf("Expected client2, got %s", selected.ID)
	}
}

// TestWeightedSelector 测试加权选择器
func TestWeightedSelector(t *testing.T) {
	weights := map[string]int{
		"client1": 5,
		"client2": 3,
		"client3": 2,
	}
	selector := NewWeightedSelector(weights)

	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	// 测试权重分布
	counts := make(map[string]int)
	iterations := 10000
	for i := 0; i < iterations; i++ {
		client, err := selector.Select(clients)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[client.ID]++
	}

	// 验证分布符合权重比例（允许 10% 误差）
	totalWeight := 10.0
	for id, weight := range weights {
		expectedRatio := float64(weight) / totalWeight
		actualRatio := float64(counts[id]) / float64(iterations)
		diff := actualRatio - expectedRatio
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.1 {
			t.Errorf("Client %s: expected ratio %.2f, got %.2f (diff %.2f)",
				id, expectedRatio, actualRatio, diff)
		}
	}
}

// TestRandomSelector 测试随机选择器
func TestRandomSelector(t *testing.T) {
	selector := NewRandomSelector()

	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	// 测试随机分布
	counts := make(map[string]int)
	iterations := 3000
	for i := 0; i < iterations; i++ {
		client, err := selector.Select(clients)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		counts[client.ID]++
	}

	// 验证每个客户端都被选中过
	for id := range clients {
		if counts[id.ID] == 0 {
			t.Errorf("Client %s was never selected", id.ID)
		}
	}

	// 验证分布相对均匀（允许 20% 误差，因为是随机的）
	expectedCount := iterations / len(clients)
	for id, count := range counts {
		diff := float64(count-expectedCount) / float64(expectedCount)
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.2 {
			t.Errorf("Client %s: expected ~%d, got %d (diff %.2f%%)",
				id, expectedCount, count, diff*100)
		}
	}
}

// TestEmptyClients 测试空客户端列表
func TestEmptyClients(t *testing.T) {
	clients := map[*Client]bool{}

	selectors := []ClientSelector{
		NewRoundRobinSelector(),
		NewLeastConnectionSelector(),
		NewWeightedSelector(nil),
		NewRandomSelector(),
	}

	for _, selector := range selectors {
		_, err := selector.Select(clients)
		if err == nil {
			t.Errorf("Expected error for empty clients, got nil")
		}
	}
}

// TestConcurrentAccess 测试并发访问
func TestConcurrentAccess(t *testing.T) {
	selector := NewRoundRobinSelector()

	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := selector.Select(clients)
				if err != nil {
					t.Errorf("Select failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()
}

// BenchmarkRoundRobinSelector 基准测试轮询选择器
func BenchmarkRoundRobinSelector(b *testing.B) {
	selector := NewRoundRobinSelector()
	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.Select(clients)
	}
}

// BenchmarkLeastConnectionSelector 基准测试最少连接选择器
func BenchmarkLeastConnectionSelector(b *testing.B) {
	selector := NewLeastConnectionSelector()
	clients := map[*Client]bool{
		createTestClient("client1", 5): true,
		createTestClient("client2", 2): true,
		createTestClient("client3", 8): true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.Select(clients)
	}
}

// BenchmarkWeightedSelector 基准测试加权选择器
func BenchmarkWeightedSelector(b *testing.B) {
	weights := map[string]int{
		"client1": 5,
		"client2": 3,
		"client3": 2,
	}
	selector := NewWeightedSelector(weights)
	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.Select(clients)
	}
}

// BenchmarkRandomSelector 基准测试随机选择器
func BenchmarkRandomSelector(b *testing.B) {
	selector := NewRandomSelector()
	clients := map[*Client]bool{
		createTestClient("client1", 0): true,
		createTestClient("client2", 0): true,
		createTestClient("client3", 0): true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		selector.Select(clients)
	}
}
