package utils

import (
	"fmt"

	"github.com/fatih/color"
)

// PrintSeparator 打印分隔线
func PrintSeparator() {
	color.Cyan("─────────────────────────────────────────────────────────────────")
}

// PrintLabelValue 打印带标签和值的格式化输出
func PrintLabelValue(icon string, label string, value interface{}) {
	color.New(color.FgGreen).Printf("%-2s %-6s", icon, label+":")
	fmt.Println(value)
}

// PrintLabelValueWithColor 使用指定颜色打印标签和值
func PrintLabelValueWithColor(icon string, label string, value interface{}, textColor *color.Color) {
	if textColor == nil {
		textColor = color.New(color.FgGreen)
	}
	textColor.Printf("%-2s %-6s", icon, label+":")
	fmt.Println(value)
}

// FormatDuration 格式化视频时长为时分秒
func FormatDuration(seconds float64) string {
	// 将毫秒转换为秒
	totalSeconds := int(seconds / 1000)

	// 计算时分秒
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	secs := totalSeconds % 60

	// 根据时长返回不同格式
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// FormatNumber 格式化数字，将大数字格式化为更易读的形式
func FormatNumber(num float64) string {
	if num >= 100000000 {
		return fmt.Sprintf("%.1f亿", num/100000000)
	} else if num >= 10000 {
		return fmt.Sprintf("%.1f万", num/10000)
	}
	return fmt.Sprintf("%.0f", num)
}
