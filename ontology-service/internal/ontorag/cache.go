// cache.go 元素向量按 spec 内容缓存（84 号评审 A2「元素向量随本体版本缓存」，86 号体检 N3 清账）：
// 元素集与向量是 spec_json 的确定性派生物——以「本体×spec 指纹」为键做进程内缓存，
// 同一 spec 反复扩展（调 top_k/profile、试跑、多次领取）不再重调 embedding 代理；
// spec 任何变更（保存/合并/导入）内容指纹即变，旧缓存自然失效，无需显式失效通道。
// 诚实边界：进程内缓存，服务重启即空（沿 ai-draft job 进程内口径）；容量 LRU 封顶防膨胀。
package ontorag

import (
	"hash/fnv"
	"sync"
)

// VectorCache 元素向量缓存（键=ontologyID+spec 指纹；LRU 淘汰，默认容量见 DefaultCacheCapacity）。
type VectorCache struct {
	mu      sync.Mutex
	cap     int
	entries map[string]cacheEntry
	lru     []string // 键的访问序（最旧在前，淘汰用）
}

// DefaultCacheCapacity 缓存本体数默认上限（单机尺度；每个条目=一 entirety 元素集+同序向量）。
const DefaultCacheCapacity = 8

type cacheEntry struct {
	elements []Element
	vectors  [][]float64
}

// NewVectorCache 构造缓存；capacity<=0 取默认。
func NewVectorCache(capacity int) *VectorCache {
	if capacity <= 0 {
		capacity = DefaultCacheCapacity
	}
	return &VectorCache{cap: capacity, entries: map[string]cacheEntry{}}
}

// SpecFingerprint spec 内容指纹（FNV-1a 64）——同文同键，任何变更即新键。
func SpecFingerprint(specJSON string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(specJSON))
	return fnv64aHex(h.Sum64())
}

func fnv64aHex(v uint64) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		out[i] = hex[v&0xf]
		v >>= 4
	}
	return string(out)
}

func cacheKey(ontologyID, fingerprint string) string { return ontologyID + "\x00" + fingerprint }

// Get 查缓存（命中即复用，访问序刷新）。
func (c *VectorCache) Get(ontologyID, fingerprint string) ([]Element, [][]float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(ontologyID, fingerprint)
	e, ok := c.entries[key]
	if !ok {
		return nil, nil, false
	}
	c.touchLocked(key)
	return e.elements, e.vectors, true
}

// Put 写缓存（超容量淘汰最旧；同键覆盖）。
func (c *VectorCache) Put(ontologyID, fingerprint string, elements []Element, vectors [][]float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(ontologyID, fingerprint)
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.cap {
		c.evictOldestLocked()
	}
	c.entries[key] = cacheEntry{elements: elements, vectors: vectors}
	c.touchLocked(key)
}

// touchLocked 把 key 移到访问序末尾（调用方持锁）。
func (c *VectorCache) touchLocked(key string) {
	for i, k := range c.lru {
		if k == key {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			break
		}
	}
	c.lru = append(c.lru, key)
}

func (c *VectorCache) evictOldestLocked() {
	if len(c.lru) == 0 {
		return
	}
	oldest := c.lru[0]
	c.lru = c.lru[1:]
	delete(c.entries, oldest)
}
