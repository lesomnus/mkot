package z

import "fmt"

func Err(err error, format string, a ...any) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), err)
}
