package repository

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FilePath validates an already decoded logical file path. Scheduler stores
// metadata, not files, so remote names must not inherit the host OS naming rules.
// Keep this remote policy aligned with DingoSpeed's POSIX repository.FilePath,
// including literal backslashes; Windows Speed rejects unrepresentable names
// at its filesystem boundary, not in Scheduler's logical metadata.
func (k Key) FilePath(name string) error {
	if k.Namespace != "huggingface" && k.Namespace != "modelscope" {
		return ValidatePath(name, true)
	}
	if len(name) > 1024 || !utf8.ValidString(name) {
		return fmt.Errorf("invalid remote file path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 {
			return fmt.Errorf("unsafe remote path segment %q", part)
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return fmt.Errorf("control character in remote file path")
			}
		}
	}
	return nil
}
