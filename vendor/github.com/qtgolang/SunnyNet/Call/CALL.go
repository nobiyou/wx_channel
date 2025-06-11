package Call

var MakeChanNum = 750
var MakeChanInit = false

//export MakeChan
func MakeChan(mun int) {
	MakeChanNum = mun
}

// 限制CALL通知函数的访问 避免耗尽资源导致崩溃
var ch = make(chan bool, MakeChanNum)
