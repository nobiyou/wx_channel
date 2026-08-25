package lifecycle

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

const wechatProcessName = "Weixin.exe"

func parseTasklistOutput(raw []byte) (bool, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff")))
	reader.FieldsPerRecord = -1
	for {
		record, err := reader.Read()
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("parse tasklist output: %w", err)
		}
		if len(record) > 0 && strings.EqualFold(strings.TrimSpace(record[0]), wechatProcessName) {
			return true, nil
		}
	}
}
