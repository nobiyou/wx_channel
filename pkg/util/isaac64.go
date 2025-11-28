package util

// Isaac64 实现微信视频号使用的 ISAAC64 伪随机数生成器
// 用于从 seed (key) 生成解密数组
type Isaac64 struct {
	randrsl [256]uint64
	randcnt uint64
	mm      [256]uint64
	aa      uint64
	bb      uint64
	cc      uint64
}

// NewIsaac64 创建新的 Isaac64 实例
func NewIsaac64(seed uint64) *Isaac64 {
	i := &Isaac64{}
	i.randrsl[0] = seed
	i.randinit(true)
	return i
}

func (i *Isaac64) randinit(flag bool) {
	var a, b, c, d, e, f, g, h uint64
	a = 0x9e3779b97f4a7c13
	b = a
	c = a
	d = a
	e = a
	f = a
	g = a
	h = a

	for j := 0; j < 4; j++ {
		a, b, c, d, e, f, g, h = i.mix(a, b, c, d, e, f, g, h)
	}

	for j := 0; j < 256; j += 8 {
		if flag {
			a += i.randrsl[j]
			b += i.randrsl[j+1]
			c += i.randrsl[j+2]
			d += i.randrsl[j+3]
			e += i.randrsl[j+4]
			f += i.randrsl[j+5]
			g += i.randrsl[j+6]
			h += i.randrsl[j+7]
		}
		a, b, c, d, e, f, g, h = i.mix(a, b, c, d, e, f, g, h)
		i.mm[j] = a
		i.mm[j+1] = b
		i.mm[j+2] = c
		i.mm[j+3] = d
		i.mm[j+4] = e
		i.mm[j+5] = f
		i.mm[j+6] = g
		i.mm[j+7] = h
	}

	if flag {
		for j := 0; j < 256; j += 8 {
			a += i.mm[j]
			b += i.mm[j+1]
			c += i.mm[j+2]
			d += i.mm[j+3]
			e += i.mm[j+4]
			f += i.mm[j+5]
			g += i.mm[j+6]
			h += i.mm[j+7]
			a, b, c, d, e, f, g, h = i.mix(a, b, c, d, e, f, g, h)
			i.mm[j] = a
			i.mm[j+1] = b
			i.mm[j+2] = c
			i.mm[j+3] = d
			i.mm[j+4] = e
			i.mm[j+5] = f
			i.mm[j+6] = g
			i.mm[j+7] = h
		}
	}

	i.isaac64()
	i.randcnt = 256
}

func (i *Isaac64) mix(a, b, c, d, e, f, g, h uint64) (uint64, uint64, uint64, uint64, uint64, uint64, uint64, uint64) {
	a -= e
	f ^= h >> 9
	h += a
	b -= f
	g ^= a << 9
	a += b
	c -= g
	h ^= b >> 23
	b += c
	d -= h
	a ^= c << 15
	c += d
	e -= a
	b ^= d >> 14
	d += e
	f -= b
	c ^= e << 20
	e += f
	g -= c
	d ^= f >> 17
	f += g
	h -= d
	e ^= g << 14
	g += h
	return a, b, c, d, e, f, g, h
}

func (i *Isaac64) isaac64() {
	i.cc++
	i.bb += i.cc

	for j := 0; j < 256; j++ {
		x := i.mm[j]
		switch j % 4 {
		case 0:
			i.aa = ^(i.aa ^ (i.aa << 21))
		case 1:
			i.aa = i.aa ^ (i.aa >> 5)
		case 2:
			i.aa = i.aa ^ (i.aa << 12)
		case 3:
			i.aa = i.aa ^ (i.aa >> 33)
		}
		i.aa += i.mm[(j+128)%256]
		y := i.mm[(x>>3)%256] + i.aa + i.bb
		i.mm[j] = y
		i.bb = i.mm[(y>>11)%256] + x
		i.randrsl[j] = i.bb
	}
}

// Generate 生成指定长度的解密数组
func (i *Isaac64) Generate(length int) []byte {
	result := make([]byte, length)
	pos := 0

	for pos < length {
		if i.randcnt == 0 {
			i.isaac64()
			i.randcnt = 256
		}
		i.randcnt--
		val := i.randrsl[i.randcnt]

		// 将 uint64 转换为 8 个字节（小端序，然后反转）
		bytes := make([]byte, 8)
		for k := 0; k < 8 && pos < length; k++ {
			bytes[k] = byte(val >> (8 * k))
		}
		// 反转字节顺序
		for k := 7; k >= 0 && pos < length; k-- {
			result[pos] = bytes[k]
			pos++
		}
	}

	return result
}

// GenerateDecryptorArray 从 seed 生成解密数组（兼容微信视频号格式）
func GenerateDecryptorArray(seed uint64, length int) []byte {
	isaac := NewIsaac64(seed)
	return isaac.Generate(length)
}
