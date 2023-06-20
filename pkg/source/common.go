package source

import (
	"fmt"
)

// TODO: Remove this?
func generateMessage(bundleName string) string {
	return fmt.Sprintf("Successfully unpacked the %s source", bundleName)
}
