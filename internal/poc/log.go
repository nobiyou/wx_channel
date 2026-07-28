package poc

type SafeLogger interface {
	Event(name string, fields map[string]any) error
}

type DiscardLogger struct{}

func (DiscardLogger) Event(string, map[string]any) error {
	return nil
}
