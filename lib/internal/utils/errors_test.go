package utils

import (
	"errors"
	"testing"
)

func TestHandleError(t *testing.T) {
	err := errors.New("测试错误")
	// 这个测试主要确保函数不会崩溃
	HandleError(err, "测试上下文")
}

func TestMust(t *testing.T) {
	var err error
	
	// 测试无错误情况
	err = nil
	Must(err, "无错误测试") // 不应该崩溃
	
	// 测试有错误情况
	err = errors.New("测试错误")
	Must(err, "有错误测试") // 应该处理错误
}

func TestErrorf(t *testing.T) {
	err := Errorf("格式化错误: %s", "测试消息")
	if err == nil {
		t.Error("Errorf应该返回错误")
	}
	if err.Error() == "" {
		t.Error("错误信息不应该为空")
	}
}

