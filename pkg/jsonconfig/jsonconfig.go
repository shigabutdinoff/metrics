package jsonconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Load читает JSON-файл path в v, поля без ключа в файле не меняются.
func Load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Seconds интервал в целых секундах, в JSON строка вида "1s".
type Seconds int64

// UnmarshalJSON разбирает строку time.ParseDuration, доли секунды отбрасывает.
func (s *Seconds) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf(`интервал %s: нужна строка вида "1s": %w`, data, err)
	}
	d, err := time.ParseDuration(str)
	if err != nil {
		return fmt.Errorf(`интервал %s: нужна строка вида "1s": %w`, data, err)
	}
	*s = Seconds(d / time.Second)
	return nil
}
