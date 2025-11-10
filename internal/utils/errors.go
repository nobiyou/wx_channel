package utils

import (
	"fmt"
	"log"

	"github.com/fatih/color"
)

// HandleError 处理错误，输出到日志和控制台
func HandleError(err error, context string) {
	if err != nil {
		log.Printf("[ERROR] %s: %v", context, err)
		color.Red("❌ %s: %v\n", context, err)
	}
}

// HandleErrorWithExit 处理错误并退出程序
func HandleErrorWithExit(err error, context string) {
	if err != nil {
		log.Printf("[FATAL] %s: %v", context, err)
		color.Red("❌ %s: %v\n", context, err)
		// 这里可以根据需要决定是否退出
	}
}

// Must 检查错误，如果有错误则处理
func Must(err error, context string) {
	if err != nil {
		HandleError(err, context)
	}
}

// MustFatal 检查错误，如果有错误则处理并退出
func MustFatal(err error, context string) {
	if err != nil {
		HandleErrorWithExit(err, context)
	}
}

// Errorf 格式化错误并处理
func Errorf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	log.Printf("[ERROR] %v", err)
	color.Red("❌ %v\n", err)
	return err
}
